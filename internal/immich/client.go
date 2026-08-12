package immich

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode"
)

type Person struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func New(baseURL, apiKey string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, http: http.DefaultClient}
}

func (c *Client) Enabled() bool { return c != nil && c.baseURL != "" && c.apiKey != "" }

func (c *Client) People(ctx context.Context) ([]Person, error) {
	var people []Person
	if err := c.getJSON(ctx, "/api/people", &people); err != nil {
		return nil, err
	}
	return people, nil
}

func (c *Client) Thumbnail(ctx context.Context, id string) (io.ReadCloser, string, error) {
	req, err := c.request(ctx, http.MethodGet, "/api/people/"+url.PathEscape(id)+"/thumbnail")
	if err != nil {
		return nil, "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, "", fmt.Errorf("immich thumbnail returned %s", resp.Status)
	}
	return resp.Body, resp.Header.Get("Content-Type"), nil
}

func (c *Client) getJSON(ctx context.Context, path string, dst any) error {
	req, err := c.request(ctx, http.MethodGet, path)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("immich returned %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

func (c *Client) request(ctx context.Context, method, path string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", c.apiKey)
	return req, nil
}

func NormalizeName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func ExactMatch(name string, people []Person) *Person {
	want := NormalizeName(name)
	var match *Person
	for i := range people {
		if NormalizeName(people[i].Name) == want {
			if match != nil {
				return nil
			}
			match = &people[i]
		}
	}
	return match
}
