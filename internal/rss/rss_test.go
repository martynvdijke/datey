package rss

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"
)

func TestRender_ValidRSS20(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	items := []Item{
		{Title: "Birthday — Dana in 3 days", Link: "http://localhost:6270/people/1", Description: "Dana's birthday"},
		{Title: "Anniversary — Sam in 1 day", Link: "http://localhost:6270/people/2"},
	}
	body, err := Render(Channel{
		Title:       "Datey — Upcoming events",
		Link:        "http://localhost:6270",
		Description: "Upcoming events tracked in Datey.",
		TTL:         360,
	}, items, now)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	if !strings.HasPrefix(string(body), xml.Header) {
		t.Errorf("feed must start with XML declaration")
	}

	var doc RSS
	if err := xml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("output is not well-formed XML: %v\n%s", err, body)
	}
	if doc.Version != "2.0" {
		t.Errorf("version = %q, want 2.0", doc.Version)
	}
	if doc.Channel.Title != "Datey — Upcoming events" {
		t.Errorf("channel title = %q", doc.Channel.Title)
	}
	if doc.Channel.TTL != 360 {
		t.Errorf("ttl = %d, want 360", doc.Channel.TTL)
	}
	if got, want := len(doc.Channel.Items), len(items); got != want {
		t.Fatalf("item count = %d, want %d", got, want)
	}
	for i, it := range doc.Channel.Items {
		if it.Title != items[i].Title {
			t.Errorf("item %d title = %q, want %q", i, it.Title, items[i].Title)
		}
		if it.Link != items[i].Link {
			t.Errorf("item %d link = %q, want %q", i, it.Link, items[i].Link)
		}
		if _, err := time.Parse(time.RFC1123Z, it.PubDate); err != nil {
			t.Errorf("item %d pubDate %q not RFC1123Z: %v", i, it.PubDate, err)
		}
		if it.GUID != it.Link {
			t.Errorf("item %d guid = %q, want link fallback %q", i, it.GUID, it.Link)
		}
	}
	if _, err := time.Parse(time.RFC1123Z, doc.Channel.LastBuildDate); err != nil {
		t.Errorf("lastBuildDate %q not RFC1123Z: %v", doc.Channel.LastBuildDate, err)
	}
	if doc.Channel.Items[0].Description != items[0].Description {
		t.Errorf("description did not round-trip")
	}
}

func TestRender_EmptyFeed(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	body, err := Render(Channel{Title: "Datey — Upcoming events", TTL: 360}, nil, now)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	var doc RSS
	if err := xml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("empty feed is not well-formed XML: %v\n%s", err, body)
	}
	if doc.Channel.Title == "" {
		t.Errorf("channel title lost")
	}
	if len(doc.Channel.Items) != 0 {
		t.Errorf("expected no items, got %d", len(doc.Channel.Items))
	}
}

func TestRender_XMLEscaping(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	title := `R & D <party> "quoted" 'single'`
	body, err := Render(Channel{Title: "Datey"}, []Item{
		{Title: title, Link: "http://x/1?a=1&b=2"},
	}, now)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	raw := string(body)
	if strings.Contains(raw, "&D <party>") || strings.Contains(raw, `"quoted"`) {
		t.Errorf("raw unescaped characters in output:\n%s", raw)
	}

	var doc RSS
	if err := xml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("escaped output not well-formed: %v\n%s", err, body)
	}
	if doc.Channel.Items[0].Title != title {
		t.Errorf("title round-trip = %q, want %q", doc.Channel.Items[0].Title, title)
	}
	if doc.Channel.Items[0].Link != "http://x/1?a=1&b=2" {
		t.Errorf("link round-trip = %q", doc.Channel.Items[0].Link)
	}
}

func TestRender_ExplicitTimestampsPreserved(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	itemPub := time.Date(2026, 8, 8, 8, 30, 0, 0, time.UTC).Format(time.RFC1123Z)
	body, err := Render(Channel{Title: "Datey"}, []Item{
		{Title: "X", Link: "http://x/1", PubDate: itemPub, GUID: "custom-guid"},
	}, now)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	var doc RSS
	if err := xml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("output not well-formed: %v", err)
	}
	if doc.Channel.Items[0].PubDate != itemPub {
		t.Errorf("explicit pubDate = %q, want %q", doc.Channel.Items[0].PubDate, itemPub)
	}
	if doc.Channel.Items[0].GUID != "custom-guid" {
		t.Errorf("explicit guid = %q, want custom-guid", doc.Channel.Items[0].GUID)
	}
}
