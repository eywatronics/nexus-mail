package app

import "testing"

func TestBadgeLabel(t *testing.T) {
	cases := []struct {
		unread int
		want   string
		why    string
	}{
		// Nothing to notice, so nothing is drawn. A badge reading zero is a
		// mark on the icon that says there is no news.
		{0, "", "an empty inbox leaves the icon alone"},
		{-3, "", "a count that went negative is a bug, not a badge"},
		{1, "1", ""},
		{99, "99", "two digits still fit the circle"},
		// Past this the exact number has stopped being information: the answer
		// to "how many" is "more than you are reading now" either way.
		{100, "99+", "three digits are a smudge at this size"},
		{4821, "99+", ""},
	}

	for _, c := range cases {
		if got := BadgeLabel(c.unread); got != c.want {
			t.Errorf("BadgeLabel(%d) = %q, want %q — %s", c.unread, got, c.want, c.why)
		}
	}
}

// The badge and the tooltip are the two places the count is shown outside the
// window, and they must not disagree about whether there is anything to show.
func TestTheBadgeAndTheTooltipAgreeOnEmptiness(t *testing.T) {
	for _, unread := range []int{-1, 0, 1, 7, 500} {
		quiet := BadgeLabel(unread) == ""
		plain := TrayTooltip(unread) == appName

		if quiet != plain {
			t.Errorf("at %d unread the badge says %q while the tooltip says %q",
				unread, BadgeLabel(unread), TrayTooltip(unread))
		}
	}
}
