package googlecontacts

import (
	"context"
	"fmt"

	"golang.org/x/oauth2"
)

var (
	Scopes   = []string{"https://www.googleapis.com/auth/contacts.readonly"}
	Endpoint = oauth2.Endpoint{
		AuthURL:  "https://accounts.google.com/o/oauth2/auth",
		TokenURL: "https://oauth2.googleapis.com/token",
	}
)

// OAuthConfig builds an oauth2.Config for Google.
func OAuthConfig(clientID, clientSecret, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       Scopes,
		Endpoint:     Endpoint,
	}
}

// AuthURL returns the URL to redirect the user to for consent.
func AuthURL(cfg *oauth2.Config, state string) string {
	return cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
}

// ExchangeCode exchanges an authorization code for a token.
func ExchangeCode(ctx context.Context, cfg *oauth2.Config, code string) (*oauth2.Token, error) {
	tok, err := cfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("oauth exchange: %w", err)
	}
	return tok, nil
}

// TokenSource returns a TokenSource that refreshes using the refresh token.
func TokenSource(ctx context.Context, cfg *oauth2.Config, refreshToken string) oauth2.TokenSource {
	tok := &oauth2.Token{RefreshToken: refreshToken}
	return cfg.TokenSource(ctx, tok)
}
