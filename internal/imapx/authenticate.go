package imapx

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-sasl"

	"nexusmail/internal/auth"
)

// authenticate presents the credential in a way this server will accept.
//
// Until now the client always sent AUTHENTICATE PLAIN. That is the right first
// choice and wrong as the only one: a server decides which mechanisms it
// offers, and on-premises Exchange in particular advertises different ones
// depending on how its IMAP4 service is configured. A server that does not
// offer PLAIN answered with a bare NO, and the reader was told "authentication
// failed" about a password that was perfectly correct.
//
// Every path below carries the same secret in the same direction, so choosing
// between them is a compatibility question and not a security one. The
// security question was already answered: the connection is encrypted before
// this function is reached.
func authenticate(ctx context.Context, c *imapclient.Client, provider auth.CredentialProvider) error {
	caps := c.Caps()

	pw, isPassword := provider.(auth.PasswordCredential)
	if !isPassword {
		// A minted token has exactly one mechanism. Saying so beats letting
		// the server answer NO to a mechanism it never advertised.
		if !caps.Has(imap.AuthCap("PLAIN")) && !supportsMechanism(caps, "XOAUTH2") {
			return fmt.Errorf(
				"imapx: the server does not offer OAuth sign-in for IMAP; it offers %s",
				describeMechanisms(caps))
		}
		client, err := provider.SASLClient(ctx)
		if err != nil {
			return err
		}
		return c.Authenticate(client)
	}

	secret, err := pw.Password(ctx)
	if err != nil {
		return err
	}
	username := pw.Username()

	switch {
	case caps.Has(imap.AuthCap("PLAIN")):
		return c.Authenticate(sasl.NewPlainClient("", username, secret))

	case supportsMechanism(caps, "LOGIN"):
		return withMechanismContext(caps, c.Authenticate(sasl.NewLoginClient(username, secret)))

	case !caps.Has(imap.CapLoginDisabled):
		// The LOGIN command rather than a SASL mechanism. The server has not
		// said it is disabled, so it is worth trying, and the credential
		// travels the same way it would have either way.
		return withMechanismContext(caps, c.Login(username, secret).Wait())

	default:
		return fmt.Errorf(
			"imapx: this server will not take a password on this connection; "+
				"it offers %s", describeMechanisms(caps))
	}
}

// withMechanismContext annotates a failure from a fallback path with what the
// server actually offered.
//
// This is the difference between a useful failure and a useless one. When a
// site has locked its Exchange down to NTLM and GSSAPI, every password the
// reader types will be rejected and none of them are wrong; being told what
// the server asked for is what turns retyping into a conversation with
// whoever configured it.
func withMechanismContext(caps imap.CapSet, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w — the server offers %s, none of which is a password "+
		"mechanism this client can use", err, describeMechanisms(caps))
}

// supportsMechanism reports whether the server advertised AUTH=<name>.
func supportsMechanism(caps imap.CapSet, name string) bool {
	return caps.Has(imap.AuthCap(name))
}

// describeMechanisms lists what the server did offer, so a failure says
// something the operator can act on.
//
// This is the whole value of the error. A site whose Exchange only advertises
// NTLM and GSSAPI needs to know that, because no amount of retyping the
// password will help and the next step is a conversation with whoever
// configured the server.
func describeMechanisms(caps imap.CapSet) string {
	var offered []string
	for c := range caps {
		if name, found := strings.CutPrefix(string(c), "AUTH="); found {
			offered = append(offered, name)
		}
	}
	if len(offered) == 0 {
		return "none"
	}
	sort.Strings(offered)
	return strings.Join(offered, ", ")
}
