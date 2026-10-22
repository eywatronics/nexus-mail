package app

import (
	"context"
	"net"
	"time"

	"nexusmail/internal/auth"
)

// DiscoveredServerDTO is what the account form fills in after a lookup.
type DiscoveredServerDTO struct {
	IMAPHost string `json:"imapHost"`
	IMAPPort int    `json:"imapPort"`
	Security string `json:"security"`
	SMTPHost string `json:"smtpHost"`
	SMTPPort int    `json:"smtpPort"`
	// Source is "srv" when the provider published the service and "guess"
	// when the answer came from conventional hostnames. The form says which,
	// because the two deserve different amounts of trust.
	Source string `json:"source"`
}

// DiscoverServer looks up the server settings for an address's domain.
//
// Bounded at five seconds: DNS is usually milliseconds, and an account form
// that waits on a dead resolver has no way to tell the person to stop waiting.
// An explicit button calls this, not every keystroke, so a typo does not become
// a DNS query.
func (s *MailService) DiscoverServer(email string) (DiscoveredServerDTO, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	found, err := auth.Discover(ctx, email, net.DefaultResolver)
	if err != nil {
		return DiscoveredServerDTO{}, err
	}
	return DiscoveredServerDTO{
		IMAPHost: found.IMAPHost,
		IMAPPort: found.IMAPPort,
		Security: string(found.Security),
		SMTPHost: found.SMTPHost,
		SMTPPort: found.SMTPPort,
		Source:   found.Source,
	}, nil
}
