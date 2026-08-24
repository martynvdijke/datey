package googlecontacts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to the Google People API.
type Client struct {
	transport Transport
	baseURL   string
}

func New(transport Transport) *Client {
	return &Client{transport: transport, baseURL: "https://people.googleapis.com"}
}

func NewWithBaseURL(transport Transport, baseURL string) *Client {
	return &Client{transport: transport, baseURL: strings.TrimRight(baseURL, "/")}
}

type Contact struct {
	ResourceName string      `json:"resourceName"`
	Names        []Name      `json:"names"`
	Birthdays    []Birthday  `json:"birthdays"`
	Biographies  []Biography `json:"biographies"`
	Metadata     *Metadata   `json:"metadata"`
}

// Metadata mirrors person.metadata; Deleted is set on tombstones returned by
// incremental (syncToken) responses.
type Metadata struct {
	Deleted bool `json:"deleted"`
}

type Name struct {
	DisplayName string `json:"displayName"`
	GivenName   string `json:"givenName"`
	FamilyName  string `json:"familyName"`
}

type Birthday struct {
	Date *Date  `json:"date"`
	Text string `json:"text"`
}

type Date struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	Day   int `json:"day"`
}

type Biography struct {
	Value string `json:"value"`
}

func (c *Contact) DisplayName() string {
	if len(c.Names) > 0 && c.Names[0].DisplayName != "" {
		return c.Names[0].DisplayName
	}
	return ""
}

func (c *Contact) BirthdayTime() *time.Time {
	for _, b := range c.Birthdays {
		if b.Date != nil && b.Date.Month >= 1 && b.Date.Month <= 12 && b.Date.Day >= 1 && b.Date.Day <= 31 {
			year := b.Date.Year
			if year == 0 {
				year = 2000
			}
			t := time.Date(year, time.Month(b.Date.Month), b.Date.Day, 0, 0, 0, 0, time.UTC)
			return &t
		}
		if b.Text != "" {
			if t := parseBirthdayText(b.Text); t != nil {
				return t
			}
		}
	}
	return nil
}

func parseBirthdayText(s string) *time.Time {
	s = strings.TrimSpace(s)
	layouts := []string{"2006-01-02", "2006/01/02", "01-02", "--01-02", "January 2", "Jan 2", "2 Jan", "2 January"}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			if t.Year() == 0 {
				t = time.Date(2000, t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
			}
			return &t
		}
	}
	return nil
}

func (c *Contact) BiographyText() string {
	if len(c.Biographies) > 0 {
		return c.Biographies[0].Value
	}
	return ""
}

type listResponse struct {
	Connections   []Contact `json:"connections"`
	NextPageToken string    `json:"nextPageToken"`
	NextSyncToken string    `json:"nextSyncToken"`
}

type ListResult struct {
	Contacts      []Contact
	NextSyncToken string
}

// ListContacts fetches all contacts via people.connections.list with pagination and sync token support.
func (c *Client) ListContacts(ctx context.Context, syncToken string) (*ListResult, error) {
	var all []Contact
	pageToken := ""
	var nextSyncToken string
	for {
		contacts, npt, nst, err := c.listPage(ctx, pageToken, syncToken)
		if err != nil {
			return nil, err
		}
		all = append(all, contacts...)
		if nst != "" {
			nextSyncToken = nst
		}
		if npt == "" {
			break
		}
		pageToken = npt
		// On paginated responses, subsequent pages should not re-send syncToken
		syncToken = ""
	}
	return &ListResult{Contacts: all, NextSyncToken: nextSyncToken}, nil
}

func (c *Client) listPage(ctx context.Context, pageToken, syncToken string) ([]Contact, string, string, error) {
	u, _ := url.Parse(c.baseURL + "/v1/people/me/connections")
	q := u.Query()
	q.Set("personFields", "names,birthdays,biographies")
	q.Set("pageSize", "1000")
	if pageToken != "" {
		q.Set("pageToken", pageToken)
	}
	if syncToken != "" && pageToken == "" {
		q.Set("syncToken", syncToken)
		q.Set("requestSyncToken", "true")
	} else if pageToken == "" {
		q.Set("requestSyncToken", "true")
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", "", err
	}
	resp, err := c.transport.Do(req)
	if err != nil {
		return nil, "", "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", "", fmt.Errorf("people.connections.list %s: %s", resp.Status, string(body))
	}
	var lr listResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return nil, "", "", err
	}
	// Ensure resourceName is set; connections already contain it
	return lr.Connections, lr.NextPageToken, lr.NextSyncToken, nil
}
