package app

import "strconv"

// badgeCap is the largest number the badge spells out.
//
// A taskbar overlay is a circle roughly sixteen pixels across. Two digits fit
// and are read at a glance; three are a smudge, and by the time somebody has
// four hundred unread the exact number has stopped being information — the
// answer to "how many" is "more than you are going to read now" either way.
const badgeCap = 99

// BadgeLabel is what the taskbar or dock badge shows for an unread count.
//
// An empty string means no badge at all rather than a badge reading zero. A
// zero is a thing to notice and there is nothing to notice: an inbox with
// nothing unread should leave the icon exactly as it was before any mail
// arrived.
//
// This lives here, beside TrayTooltip, so the two places the unread count is
// shown outside the window agree on what the number means — inbox folders
// only, as counted by UnreadCount.
func BadgeLabel(unread int) string {
	switch {
	case unread <= 0:
		return ""
	case unread > badgeCap:
		return strconv.Itoa(badgeCap) + "+"
	default:
		return strconv.Itoa(unread)
	}
}
