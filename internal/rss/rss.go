// Package rss renders RSS 2.0 documents for the public upcoming-events feed.
//
// The generator is intentionally small: it marshals a fixed set of structs
// with encoding/xml so all text values are escaped correctly, and fills in
// publication timestamps. No external dependency is required.
package rss

import (
	"encoding/xml"
	"time"
)

// Item is a single RSS 2.0 <item> element.
type Item struct {
	// Title is the human-readable item title (e.g. "Birthday — Dana in 3 days").
	Title string `xml:"title"`
	// Link is the absolute URL of the related person page.
	Link string `xml:"link"`
	// Description is an optional short item description; empty means omitted.
	Description string `xml:"description,omitempty"`
	// GUID is the unique identifier for the item. When omitted, it defaults
	// to the Link so feed readers can track the item across polls.
	GUID string `xml:"guid,omitempty"`
	// PubDate is the item publication date in RFC 1123 format. When empty,
	// Render fills it with the feed generation time.
	PubDate string `xml:"pubDate"`
}

// Channel is the RSS 2.0 <channel> element.
type Channel struct {
	Title         string `xml:"title"`
	Link          string `xml:"link"`
	Description   string `xml:"description"`
	Language      string `xml:"language,omitempty"`
	LastBuildDate string `xml:"lastBuildDate"`
	// TTL is the number of minutes readers should cache the feed before
	// polling again.
	TTL int `xml:"ttl"`
	// Items is the list of feed items; may be empty for a valid feed.
	Items []Item `xml:"item"`
}

// RSS is the root RSS 2.0 document element.
type RSS struct {
	XMLName xml.Name `xml:"rss"`
	Version string   `xml:"version,attr"`
	Channel Channel  `xml:"channel"`
}

// Render builds a complete RSS 2.0 document. Any item without a PubDate gets
// the feed generation time; a channel without a LastBuildDate does too.
// Items with an empty GUID fall back to their Link.
func Render(channel Channel, items []Item, now time.Time) ([]byte, error) {
	pub := now.Format(time.RFC1123Z)
	if channel.LastBuildDate == "" {
		channel.LastBuildDate = pub
	}
	for i := range items {
		if items[i].PubDate == "" {
			items[i].PubDate = pub
		}
		if items[i].GUID == "" {
			items[i].GUID = items[i].Link
		}
	}
	channel.Items = items

	doc := RSS{
		Version: "2.0",
		Channel: channel,
	}

	body, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(xml.Header)+len(body)+1)
	out = append(out, xml.Header...)
	out = append(out, body...)
	out = append(out, '\n')
	return out, nil
}
