package sync

import (
	"context"
	"errors"
	"net"
	"strings"
)

// ErrorClass drives how a failure is handled, per the design doc's error
// table. The classes exist because the four cases need genuinely different
// responses: retrying an auth failure forever is as wrong as giving up on a
// dropped connection.
type ErrorClass int

const (
	// ClassTransient covers network blips: retry silently with backoff.
	ClassTransient ErrorClass = iota
	// ClassAuth means credentials failed. Refresh the token first; if that
	// fails too, put the account into "sign-in required" and tell the user.
	ClassAuth
	// ClassProtocol means the server said something we cannot act on. Log it,
	// quarantine the affected folder, keep the other folders working.
	ClassProtocol
	// ClassPermanent means retrying cannot help. Tell the user and stop.
	ClassPermanent
)

func (c ErrorClass) String() string {
	switch c {
	case ClassTransient:
		return "transient"
	case ClassAuth:
		return "auth"
	case ClassProtocol:
		return "protocol"
	default:
		return "permanent"
	}
}

// authMarkers are substrings that identify a credential failure. invalid_grant
// is the important one: it is what a revoked or expired refresh token returns,
// and it means the user must sign in again rather than the app retrying.
var authMarkers = []string{
	"authentication",
	"xoauth2",
	"invalid_grant",
	"refresh token",
	"login failed",
	"authenticationfailed",
	"credentials",
}

var transientMarkers = []string{
	"connection reset",
	"broken pipe",
	"connection refused",
	"no such host",
	"timeout",
	"timed out",
	"eof",
}

var protocolMarkers = []string{
	"fetch",
	"select",
	"list failed",
	"bad command",
	"parse error",
	"unexpected response",
}

// Classify sorts an error into one of the four handling classes.
func Classify(err error) ErrorClass {
	if err == nil {
		return ClassTransient
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ClassTransient
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return ClassTransient
	}

	msg := strings.ToLower(err.Error())

	// Auth is checked first: an authentication failure often arrives wrapped
	// in a message that also mentions the command that triggered it, and
	// misreading it as a protocol error would retry forever against a
	// credential that will never work.
	if containsAny(msg, authMarkers) {
		return ClassAuth
	}
	if containsAny(msg, transientMarkers) {
		return ClassTransient
	}
	if containsAny(msg, protocolMarkers) {
		return ClassProtocol
	}
	return ClassPermanent
}

func containsAny(s string, markers []string) bool {
	for _, m := range markers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}
