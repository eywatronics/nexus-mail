package auth

import (
	"fmt"

	"github.com/emersion/go-sasl"
)

// xoauth2Client implements the XOAUTH2 SASL mechanism used by Gmail and
// Exchange Online for IMAP and SMTP.
//
// go-sasl ships PLAIN, LOGIN, ANONYMOUS and OAUTHBEARER (RFC 7628), but not
// XOAUTH2 — which is what both providers actually require for these protocols,
// so the mechanism lives here.
//
// Wire format, per Microsoft's and Google's documentation:
//
//	user=<username>\x01auth=Bearer <token>\x01\x01
type xoauth2Client struct {
	username string
	token    string
}

// NewXOAUTH2Client returns a SASL client for the XOAUTH2 mechanism.
func NewXOAUTH2Client(username, token string) sasl.Client {
	return &xoauth2Client{username: username, token: token}
}

func (c *xoauth2Client) Start() (mech string, ir []byte, err error) {
	ir = fmt.Appendf(nil, "user=%s\x01auth=Bearer %s\x01\x01", c.username, c.token)
	return "XOAUTH2", ir, nil
}

// Next is reached only when the server rejects the token: it sends a base64
// JSON error challenge and waits for an empty response before failing the
// command. Returning an error here gives the caller a usable message instead
// of an opaque protocol failure — and the challenge JSON names the missing
// scope, which is exactly what a misconfigured OAuth client needs to hear.
func (c *xoauth2Client) Next(challenge []byte) ([]byte, error) {
	return nil, fmt.Errorf("auth: XOAUTH2 rejected by server: %s", challenge)
}
