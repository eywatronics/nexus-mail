package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SettingsDTO is everything config.json holds, as the window sees it.
//
// Every one of these used to be editable only by finding a JSON file in a
// directory the app never names and editing it by hand. That is a reasonable
// thing to support and an unreasonable thing to require: the OAuth client ID
// in particular is the first thing a new user has to set, and "open
// %APPDATA%\nexus-mail\config.json" is not an answer a mail client should
// give.
//
// Seconds and days rather than durations, because that is what the file holds
// and what a person types.
type SettingsDTO struct {
	GoogleClientID    string `json:"googleClientId"`
	MicrosoftClientID string `json:"microsoftClientId"`
	OAuthRedirectPort int    `json:"oauthRedirectPort"`

	RetentionDays        int `json:"retentionDays"`
	RetentionMaxMessages int `json:"retentionMaxMessages"`

	NotificationPreview bool `json:"notificationPreview"`
	UndoWindowSeconds   int  `json:"undoWindowSeconds"`

	// StartAtLogin is the one setting here that is not in config.json.
	//
	// It lives in the operating system — a registry value, a launch agent, a
	// desktop file — and the operating system is its source of truth. Writing a
	// copy to config.json would make the file disagree with reality the moment
	// somebody turned it off in Task Manager, and the file would win on the
	// next start.
	StartAtLogin bool `json:"startAtLogin"`
	// StartAtLoginAvailable is false when the setting could not be read.
	// "Off" and "we do not know" are different answers, and the window shows
	// the switch disabled rather than showing the first when the second is true.
	StartAtLoginAvailable bool `json:"startAtLoginAvailable"`
}

// Settings returns what is in force right now.
//
// The live values, not a re-read of the file. They are the same until somebody
// edits the file by hand while the app is running, and in that case what the
// window should show is what the app is actually doing.
func (s *MailService) Settings() SettingsDTO {
	// Asked before the lock is taken. This one reaches the operating system,
	// and holding a lock the whole service reads across a call that can block
	// is how an unrelated screen freezes.
	startAtLogin, startAtLoginAvailable := s.startAtLogin()

	s.settingsMu.RLock()
	defer s.settingsMu.RUnlock()

	return SettingsDTO{
		StartAtLogin:          startAtLogin,
		StartAtLoginAvailable: startAtLoginAvailable,
		GoogleClientID:        s.cfg.GoogleClientID,
		MicrosoftClientID:     s.cfg.MicrosoftClientID,
		OAuthRedirectPort:     s.cfg.OAuthRedirectPort,
		RetentionDays:         int(s.liveRetention.MaxAge / (24 * time.Hour)),
		RetentionMaxMessages:  s.liveRetention.MaxMessages,
		NotificationPreview:   s.liveNotificationPreview,
		UndoWindowSeconds:     int(s.liveUndoWindow / time.Second),
	}
}

// UpdateSettings validates, writes config.json, and applies what can be
// applied without a restart.
//
// Written first, applied second. A setting that took effect and then failed to
// save would come back changed on the next start, which is the one outcome
// worse than not saving at all — the user would believe they had set it.
func (s *MailService) UpdateSettings(next SettingsDTO) error {
	if err := validateSettings(next); err != nil {
		return err
	}
	if s.cfg.DataDir == "" {
		return fmt.Errorf("app: no data directory configured to save settings in")
	}

	if err := writeConfigFile(s.cfg.DataDir, next); err != nil {
		return err
	}

	s.settingsMu.Lock()
	s.cfg.GoogleClientID = next.GoogleClientID
	s.cfg.MicrosoftClientID = next.MicrosoftClientID
	s.cfg.OAuthRedirectPort = next.OAuthRedirectPort
	s.liveNotificationPreview = next.NotificationPreview
	s.liveUndoWindow = time.Duration(next.UndoWindowSeconds) * time.Second
	s.liveRetention.MaxAge = time.Duration(next.RetentionDays) * 24 * time.Hour
	s.liveRetention.MaxMessages = next.RetentionMaxMessages
	retention := s.liveRetention
	s.settingsMu.Unlock()

	// The engine keeps its own copy, because the purge runs inside a sync pass
	// where reaching back into the service would be a layer inversion.
	s.engine.SetRetention(retention)

	// Last, and its failure does not undo the rest.
	//
	// This one lives outside config.json, so there is nothing for it to be
	// inconsistent with. Running it first would mean a locked registry hive
	// losing the user their retention change as well, which is a strange way
	// to handle a failure in an unrelated setting.
	return s.applyStartAtLogin(next.StartAtLogin)
}

// SettingsNeedRestart names the settings that will not take effect until the
// app is restarted, so the window can say so rather than leave the user
// wondering why nothing changed.
//
// The OAuth values are captured by the dialer when the sync engine is built,
// which happens before this service exists. Threading a live accessor through
// would buy a restart nobody minds, at the cost of a moving part in the one
// path that holds credentials.
func SettingsNeedRestart() []string {
	return []string{"googleClientId", "microsoftClientId", "oauthRedirectPort"}
}

func validateSettings(s SettingsDTO) error {
	if s.OAuthRedirectPort < 0 || s.OAuthRedirectPort > 65535 {
		return fmt.Errorf("app: oauthRedirectPort is %d; it has to be a port number, or 0 for any",
			s.OAuthRedirectPort)
	}
	// Negative limits are refused rather than clamped, the same as when the
	// file is read: clamping a typo to zero would turn "I set a limit" into
	// "keep everything", and the user would find out when the disk filled.
	if s.RetentionDays < 0 {
		return fmt.Errorf("app: retentionDays is %d; it cannot be negative (0 keeps everything)",
			s.RetentionDays)
	}
	if s.RetentionMaxMessages < 0 {
		return fmt.Errorf("app: retentionMaxMessages is %d; it cannot be negative (0 keeps everything)",
			s.RetentionMaxMessages)
	}
	if s.UndoWindowSeconds < 0 {
		return fmt.Errorf("app: undoWindowSeconds is %d; it cannot be negative (0 turns undo off)",
			s.UndoWindowSeconds)
	}
	return nil
}

// writeConfigFile replaces config.json atomically.
//
// Written to a neighbouring temporary file and renamed, so a crash or a full
// disk halfway through leaves the old file intact. A half-written config.json
// is a client that will not start, and the settings screen is not worth that.
func writeConfigFile(dir string, s SettingsDTO) error {
	fc := fileConfig{
		GoogleClientID:       s.GoogleClientID,
		MicrosoftClientID:    s.MicrosoftClientID,
		OAuthRedirectPort:    s.OAuthRedirectPort,
		RetentionDays:        &s.RetentionDays,
		RetentionMaxMessages: &s.RetentionMaxMessages,
		NotificationPreview:  &s.NotificationPreview,
		UndoWindowSeconds:    &s.UndoWindowSeconds,
	}

	encoded, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')

	path := filepath.Join(dir, configFileName)
	temp := path + ".tmp"

	if err := os.WriteFile(temp, encoded, 0o600); err != nil {
		return fmt.Errorf("app: writing %s: %w", configFileName, err)
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("app: replacing %s: %w", configFileName, err)
	}
	return nil
}
