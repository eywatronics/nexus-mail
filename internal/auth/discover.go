package auth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"

	"nexusmail/internal/model"
)

// Resolver is the part of net.Resolver this package needs.
//
// An interface so the lookup rules can be tested without DNS: a test that
// reaches the network is a test that fails when the network does, and the
// interesting cases (SRV present, MX but no SRV, nothing at all) are not the
// ones a real domain happens to provide.
type Resolver interface {
	LookupSRV(ctx context.Context, service, proto, name string) (string, []*net.SRV, error)
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
	LookupHost(ctx context.Context, host string) ([]string, error)
}

// DiscoveredServer is what to put in an account's server fields.
type DiscoveredServer struct {
	IMAPHost string
	IMAPPort int
	Security model.ConnectionSecurity
	SMTPHost string
	SMTPPort int
	// Source says where the answer came from: "srv", "mx", or "guess".
	// The window shows it because a guess and a published record deserve
	// different amounts of trust.
	Source string
}

// Discover finds IMAP and SMTP settings for an address's domain.
//
// DNS first, in the order that makes the answer strongest:
//
//  1. SRV records. The provider published the service and its port; this is
//     the only answer that is not a guess.
//  2. MX records, used only to find a hostname worth testing. The MX itself is
//     a mail exchanger, not an IMAP server, so it is not used directly.
//  3. The conventional names imap.<domain> and mail.<domain>, if they resolve.
//
// No ISPDB. Fetching a domain's settings from Mozilla's server tells a third
// party which providers this person is configuring, which is the same reason
// this client does not use a favicon service. A failed lookup is recoverable;
// a leaked address book is not.
func Discover(ctx context.Context, email string, resolver Resolver) (DiscoveredServer, error) {
	domain, err := domainOf(email)
	if err != nil {
		return DiscoveredServer{}, err
	}

	var out DiscoveredServer
	if rec := firstSRV(ctx, resolver, "imaps", domain); rec != nil {
		out.IMAPHost = cleanHost(rec.Target)
		out.IMAPPort = portOr(rec.Port, 993)
		out.Security = model.SecurityTLS
		out.Source = "srv"
	} else if rec := firstSRV(ctx, resolver, "imap", domain); rec != nil {
		out.IMAPHost = cleanHost(rec.Target)
		out.IMAPPort = portOr(rec.Port, 143)
		out.Security = model.SecuritySTARTTLS
		out.Source = "srv"
	}

	// SMTP is optional: an account can receive mail while its provider's
	// submission service is not published, and the window lets the person fill
	// it in by hand.
	if rec := firstSRV(ctx, resolver, "submissions", domain); rec != nil {
		out.SMTPHost = cleanHost(rec.Target)
		out.SMTPPort = portOr(rec.Port, 587)
	} else if rec := firstSRV(ctx, resolver, "submission", domain); rec != nil {
		out.SMTPHost = cleanHost(rec.Target)
		out.SMTPPort = portOr(rec.Port, 587)
	} else if rec := firstSRV(ctx, resolver, "smtps", domain); rec != nil {
		out.SMTPHost = cleanHost(rec.Target)
		out.SMTPPort = portOr(rec.Port, 465)
	}
	if out.SMTPHost == "" {
		out.SMTPHost, out.SMTPPort = guessSMTP(ctx, resolver, domain)
	}

	if out.IMAPHost != "" {
		return out, nil
	}

	// No SRV. An MX record at least proves the domain receives mail, and its
	// target is sometimes the same host as the mail server. Only names this
	// code is willing to guess are tested; an MX belonging to a mail provider
	// is not an IMAP host for this domain.
	mx, err := resolver.LookupMX(ctx, domain)
	if err != nil && !isNotFound(err) {
		return DiscoveredServer{}, fmt.Errorf("auth: looking up mail servers for %s: %w", domain, err)
	}
	if len(mx) > 0 {
		sort.Slice(mx, func(i, j int) bool {
			if mx[i].Pref != mx[j].Pref {
				return mx[i].Pref < mx[j].Pref
			}
			return mx[i].Host < mx[j].Host
		})
		for _, candidate := range []string{"imap." + domain, "mail." + domain} {
			if hostResolves(ctx, resolver, candidate) {
				out.IMAPHost = candidate
				out.IMAPPort = 993
				out.Security = model.SecurityTLS
				out.Source = "guess"
				break
			}
		}
		if out.IMAPHost != "" {
			if out.SMTPHost == "" && hostResolves(ctx, resolver, "smtp."+domain) {
				out.SMTPHost = "smtp." + domain
				out.SMTPPort = 587
			}
			return out, nil
		}
	}

	return DiscoveredServer{}, fmt.Errorf(
		"auth: no IMAP server found for %s; ask your provider or IT for the server name", domain)
}

// guessSMTP is the same conventional-name guess for the sending server.
// Empty when nothing resolves; the window leaves those fields for the person
// to fill rather than inventing a hostname that will fail at send time.
func guessSMTP(ctx context.Context, resolver Resolver, domain string) (string, int) {
	for _, candidate := range []string{"smtp." + domain, "mail." + domain} {
		if hostResolves(ctx, resolver, candidate) {
			return candidate, 587
		}
	}
	return "", 0
}

func domainOf(email string) (string, error) {
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return "", fmt.Errorf("auth: %q is not an email address", email)
	}
	domain := strings.ToLower(strings.TrimSpace(email[at+1:]))
	if domain == "" || strings.ContainsAny(domain, " \t/") {
		return "", fmt.Errorf("auth: %q is not a domain", domain)
	}
	return strings.TrimSuffix(domain, "."), nil
}

// firstSRV returns the record a client should try first: lowest priority, and
// highest weight within it. RFC 2782's weighting is a random choice; this is a
// deterministic ordering so the same account gets the same answer twice, which
// matters more here than spreading load.
func firstSRV(ctx context.Context, resolver Resolver, service, domain string) *net.SRV {
	_, records, err := resolver.LookupSRV(ctx, service, "tcp", domain)
	if err != nil || len(records) == 0 {
		return nil
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].Priority != records[j].Priority {
			return records[i].Priority < records[j].Priority
		}
		if records[i].Weight != records[j].Weight {
			return records[i].Weight > records[j].Weight
		}
		return records[i].Target < records[j].Target
	})
	for _, rec := range records {
		if cleanHost(rec.Target) == "" {
			continue
		}
		return rec
	}
	return nil
}

func hostResolves(ctx context.Context, resolver Resolver, host string) bool {
	addrs, err := resolver.LookupHost(ctx, host)
	return err == nil && len(addrs) > 0
}

func cleanHost(host string) string {
	host = strings.TrimSpace(host)
	host = strings.TrimSuffix(host, ".")
	if host == "" || host == "." {
		return ""
	}
	return host
}

func portOr(port uint16, fallback int) int {
	if port == 0 {
		return fallback
	}
	return int(port)
}

func isNotFound(err error) bool {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return dnsErr.IsNotFound
	}
	return false
}
