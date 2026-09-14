package app

import (
	"context"
	"fmt"
)

// UnreadCount is how many unread messages the accounts' inboxes hold, summed.
//
// Read from the folder rows rather than counted over messages. The number
// there is what the server reported unseen at the last pass, which is the same
// number every other client shows, and it costs one small query instead of a
// scan over a mailbox that the retention window allows to reach twenty-five
// thousand rows.
//
// The consequence is a lag: marking a message read locally does not drop the
// count until the change has reached the server and the next pass has read the
// status back. Seconds, in practice, and the alternative was a second
// definition of "unread" living in SQL next to model.Message.HasFlag.
//
// Inboxes only. A server-side rule that files mail into a project folder has
// already decided it is not urgent, and counting it here would make the badge
// a number nobody acts on.
func (s *MailService) UnreadCount() (int, error) {
	ctx := context.Background()

	accounts, err := s.store.ListAccounts(ctx)
	if err != nil {
		return 0, err
	}

	total := 0
	for _, acct := range accounts {
		folders, err := s.store.ListFolders(ctx, acct.ID)
		if err != nil {
			return 0, err
		}
		for _, f := range folders {
			// IsInbox rather than a path comparison in SQL: INBOX is the one
			// mailbox name IMAP defines as case-insensitive, and there is
			// already one place that knows it.
			if f.IsInbox() {
				total += f.UnreadCount
			}
		}
	}
	return total, nil
}

// TrayTooltip is what the system tray icon says on hover.
//
// The count is in the tooltip rather than drawn onto the icon. A number
// rendered into a 16-pixel tray glyph is unreadable at two digits and absent
// at three, and Windows offers no badge for tray icons the way a dock does —
// so the honest place for it is the text.
func TrayTooltip(unread int) string {
	switch {
	case unread <= 0:
		return appName
	case unread == 1:
		return appName + " — 1 unread"
	default:
		return fmt.Sprintf("%s — %d unread", appName, unread)
	}
}

// appName is the product name as it appears outside the window: in the tray
// tooltip, in notifications, in the taskbar.
const appName = "Nexus Mail"
