package auth

import (
	"strings"

	"nexusmail/internal/model"
)

// Preset holds server settings for a well-known mail domain, so the account
// wizard can skip asking for hosts and ports.
type Preset struct {
	IMAPHost string
	IMAPPort int
	SMTPHost string
	SMTPPort int
	Provider model.Provider
}

// ExchangeOnlinePreset is where every Microsoft work or school account
// answers, including the countless custom domains that are not in the table
// below.
var ExchangeOnlinePreset = Preset{
	IMAPHost: "outlook.office365.com", IMAPPort: 993,
	SMTPHost: "smtp.office365.com", SMTPPort: 587,
	Provider: model.ProviderMicrosoft,
}

// GmailPreset covers Gmail and Google Workspace custom domains alike.
var GmailPreset = Preset{
	IMAPHost: "imap.gmail.com", IMAPPort: 993,
	SMTPHost: "smtp.gmail.com", SMTPPort: 587,
	Provider: model.ProviderGoogle,
}

var presets = map[string]Preset{
	"gmail.com":      GmailPreset,
	"googlemail.com": GmailPreset,

	"outlook.com":   ExchangeOnlinePreset,
	"hotmail.com":   ExchangeOnlinePreset,
	"live.com":      ExchangeOnlinePreset,
	"msn.com":       ExchangeOnlinePreset,
	"office365.com": ExchangeOnlinePreset,

	"yandex.com":    {"imap.yandex.com", 993, "smtp.yandex.com", 465, model.ProviderGeneric},
	"yandex.com.tr": {"imap.yandex.com.tr", 993, "smtp.yandex.com.tr", 465, model.ProviderGeneric},
	"zoho.com":      {"imap.zoho.com", 993, "smtp.zoho.com", 465, model.ProviderGeneric},
	"zoho.eu":       {"imap.zoho.eu", 993, "smtp.zoho.eu", 465, model.ProviderGeneric},
	"fastmail.com":  {"imap.fastmail.com", 993, "smtp.fastmail.com", 465, model.ProviderGeneric},
	"icloud.com":    {"imap.mail.me.com", 993, "smtp.mail.me.com", 587, model.ProviderGeneric},
	"me.com":        {"imap.mail.me.com", 993, "smtp.mail.me.com", 587, model.ProviderGeneric},
	"aol.com":       {"imap.aol.com", 993, "smtp.aol.com", 465, model.ProviderGeneric},
	"yahoo.com":     {"imap.mail.yahoo.com", 993, "smtp.mail.yahoo.com", 465, model.ProviderGeneric},
}

// PresetFor looks up settings by the address's domain. It reports false for
// unknown or malformed addresses; the caller must then ask the user for hosts
// and ports. Corporate domains are the common case for a false result — they
// cannot be guessed, only asked for or discovered.
func PresetFor(email string) (Preset, bool) {
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return Preset{}, false
	}
	domain := strings.ToLower(email[at+1:])
	p, ok := presets[domain]
	return p, ok
}
