package auth

import "golang.org/x/oauth2"

// GoogleEndpoint returns Google's OAuth 2.0 endpoints.
func GoogleEndpoint() oauth2.Endpoint {
	return oauth2.Endpoint{
		AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL: "https://oauth2.googleapis.com/token",
	}
}

// MicrosoftEndpoint returns the Microsoft identity platform endpoints for the
// "common" authority, which serves both work/school and personal accounts.
// Outlook.com addresses and Microsoft 365 tenants both sign in here.
func MicrosoftEndpoint() oauth2.Endpoint {
	return oauth2.Endpoint{
		AuthURL:  "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
		TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token",
	}
}

// GoogleScopes returns the scopes needed for full IMAP access plus the address
// of the signed-in user.
//
// https://mail.google.com/ is a restricted scope: publishing an app that asks
// for it requires an annual CASA Tier 2 assessment. Users supply their own
// client ID in testing mode instead, which is why this project needs no such
// assessment.
func GoogleScopes() []string {
	return []string{
		"https://mail.google.com/",
		"https://www.googleapis.com/auth/userinfo.email",
	}
}

// MicrosoftScopes returns the scopes for IMAP and SMTP against Exchange
// Online, using Microsoft's documented protocol scope strings.
//
// offline_access is what makes refresh tokens available; without it the
// account stops working an hour after sign-in.
func MicrosoftScopes() []string {
	return []string{
		"https://outlook.office.com/IMAP.AccessAsUser.All",
		"https://outlook.office.com/SMTP.Send",
		"offline_access",
	}
}
