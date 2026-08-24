package googlecontacts

import (
	"net/http"

	"golang.org/x/oauth2"
)

// Transport performs HTTP requests.
type Transport interface {
	Do(req *http.Request) (*http.Response, error)
}

// OAuth2Transport attaches an OAuth2 bearer token to every request.
type OAuth2Transport struct {
	TokenSource oauth2.TokenSource
	Base        *http.Client
}

func (t *OAuth2Transport) Do(req *http.Request) (*http.Response, error) {
	client := t.Base
	if client == nil {
		client = http.DefaultClient
	}
	if t.TokenSource != nil {
		tok, err := t.TokenSource.Token()
		if err != nil {
			return nil, err
		}
		tok.SetAuthHeader(req)
	}
	return client.Do(req)
}

// StaticTokenTransport is used in tests.
type StaticTokenTransport struct {
	Token string
	Base  *http.Client
}

func (t *StaticTokenTransport) Do(req *http.Request) (*http.Response, error) {
	if t.Token != "" {
		req.Header.Set("Authorization", "Bearer "+t.Token)
	}
	client := t.Base
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(req)
}
