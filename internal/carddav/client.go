package carddav

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// multistatus is the parsed <multistatus> response of a PROPFIND or REPORT.
type multistatus struct {
	Responses []response `xml:"DAV: response"`
}

// response is a single <response> item. Raw holds the inner XML of the
// response element (href plus all propstat blocks) so callers can extract
// any-namespace properties (e.g. carddav:addressbook-home-set).
type response struct {
	Raw string `xml:",innerxml"`
}

// syncCollection is the <sync-collection> REPORT body. sync-collection,
// sync-token and sync-level are all DAV: namespace elements (RFC 6578).
type syncCollection struct {
	XMLName   xml.Name `xml:"DAV: sync-collection"`
	SyncToken string   `xml:"DAV: sync-token"`
	SyncLevel string   `xml:"DAV: sync-level"`
	Prop      []string `xml:"DAV: prop"`
}

// syncReportResponse is a <response> inside a sync-collection REPORT result.
// Each entry is either a changed resource (with a status of 200) or a
// deletion (status 404).
type syncReportResponse struct {
	Href   string `xml:"DAV: href"`
	Status string `xml:"DAV: status"`
}

// syncReport holds a parsed sync-collection REPORT result including the
// returned sync-token.
type syncReport struct {
	Responses []syncReportResponse `xml:"DAV: response"`
	SyncToken string               `xml:"DAV: sync-token"`
}

// Client is a minimal CardDAV client for a single address book. It performs
// discovery (well-known + PROPFIND), sync-collection REPORTs and object
// GET/PUT/DELETE against an address book URL.
type Client struct {
	transport Transport
	// addressBookURL is the resolved collection URL (the URL passed to New,
	// which is treated as already resolved; call Discover to resolve it).
	addressBookURL string
}

// New returns a Client bound to the given address book URL using t for
// requests. The URL is used as-is; call Discover when the configured URL is a
// server origin or well-known endpoint rather than a resolved collection.
func New(addressBookURL string, t Transport) *Client {
	return &Client{transport: t, addressBookURL: strings.TrimRight(addressBookURL, "/")}
}

// Discover resolves a CardDAV address book from a base URL or well-known
// endpoint. It first tries {base}/.well-known/carddav (and the legacy
// {base}/.well-known/caldav fallback), following redirects, then falls back
// to PROPFIND-ing {base} for an addressbook-home-set. Returns the resolved
// collection URL.
func (c *Client) Discover(ctx context.Context) (string, error) {
	base := c.addressBookURL
	wellKnownCandidates := []string{
		base + "/.well-known/carddav",
		base + "/.well-known/caldav",
	}
	for _, wk := range wellKnownCandidates {
		resolved, err := c.probeWellKnown(ctx, wk)
		if err == nil && resolved != "" {
			return resolved, nil
		}
	}

	// Fall back: PROPFIND the base for addressbook-home-set and use the first
	// addressbook child of the first home.
	home, err := c.findAddressbookHomeSet(ctx, base)
	if err != nil {
		return "", fmt.Errorf("carddav: discovery failed: %w", err)
	}
	book, err := c.findFirstAddressBook(ctx, home)
	if err != nil {
		return "", fmt.Errorf("carddav: no addressbook found under %s: %w", home, err)
	}
	return book, nil
}

// probeWellKnown requests a well-known URL, following redirects, and returns
// the final URL if it points at something DAV-ish (or just the resolved URL).
func (c *Client) probeWellKnown(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/calendar, text/vcard, text/directory, */*")
	resp, err := c.transport.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	// A successful well-known probe typically returns 302 to the DAV root.
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return resp.Request.URL.String(), nil
	}
	return "", fmt.Errorf("well-known probe returned %s", resp.Status)
}

// responseFragment wraps a raw <response> inner-XML fragment with the DAV and
// carddav namespace declarations so it can be parsed standalone (the original
// declarations live on the multistatus root).
const responseFragment = `<d:response xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav">%s</d:response>`

// xmlElem is a generic XML tree node used to walk property fragments by local
// name regardless of namespace prefix.
type xmlElem struct {
	XMLName  xml.Name
	Value    string   `xml:",chardata"`
	Children []xmlElem `xml:",any"`
}

// findText returns the trimmed text of the first descendant element whose
// local name matches localName, searching depth-first.
func findText(elems []xmlElem, localName string) string {
	for _, e := range elems {
		if e.XMLName.Local == localName {
			return strings.TrimSpace(e.Value)
		}
		if v := findText(e.Children, localName); v != "" {
			return v
		}
	}
	return ""
}

// hasElement reports whether any descendant element (including the given
// children) has the given local name.
func hasElement(elems []xmlElem, localName string) bool {
	for _, e := range elems {
		if e.XMLName.Local == localName {
			return true
		}
		if hasElement(e.Children, localName) {
			return true
		}
	}
	return false
}

// findAddressbookHomeSet PROPFINDs the base URL for the addressbook-home-set
// property.
func (c *Client) findAddressbookHomeSet(ctx context.Context, url string) (string, error) {
	body := `<?xml version="1.0" encoding="utf-8" ?>
<d:propfind xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav">
  <d:prop>
    <card:addressbook-home-set/>
  </d:prop>
</d:propfind>`

	ms, err := c.propfind(ctx, url, body)
	if err != nil {
		return "", err
	}
	for _, r := range ms.Responses {
		if home := extractPropValue(r.Raw, "addressbook-home-set"); home != "" {
			return resolveRelative(url, home), nil
		}
	}
	return "", fmt.Errorf("no addressbook-home-set found at %s", url)
}

// findFirstAddressBook PROPFINDs a home URL for addressbook collection
// children and returns the href of the first addressbook resource.
func (c *Client) findFirstAddressBook(ctx context.Context, home string) (string, error) {
	body := `<?xml version="1.0" encoding="utf-8" ?>
<d:propfind xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav">
  <d:prop>
    <d:resourcetype/>
  </d:prop>
</d:propfind>`

	ms, err := c.propfind(ctx, home, body)
	if err != nil {
		return "", err
	}
	for _, r := range ms.Responses {
		href := extractPropValue(r.Raw, "href")
		if strings.Contains(href, "/") && strings.HasSuffix(strings.ToLower(href), "/") {
			// Collections end with a slash; probe it for the addressbook
			// resource type.
			book, err := c.isAddressBook(ctx, resolveRelative(home, href))
			if err == nil && book {
				return resolveRelative(home, href), nil
			}
		}
	}
	return "", fmt.Errorf("no addressbook collection under %s", home)
}

// isAddressBook PROPFINDs a URL for its resourcetype to check for the
// addressbook element. Only the response matching the requested href is
// considered: a depth-1 PROPFIND of a home may return many children in one
// multistatus, and any addressbook among them would otherwise be attributed
// to the URL being probed.
func (c *Client) isAddressBook(ctx context.Context, probeURL string) (bool, error) {
	body := `<?xml version="1.0" encoding="utf-8" ?>
<d:propfind xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav">
  <d:prop>
    <d:resourcetype/>
  </d:prop>
</d:propfind>`

	ms, err := c.propfind(ctx, probeURL, body)
	if err != nil {
		return false, err
	}
	u, err := url.Parse(probeURL)
	if err != nil {
		return false, err
	}
	wantPath := strings.TrimRight(u.Path, "/")
	for _, r := range ms.Responses {
		href := strings.TrimRight(extractPropValue(r.Raw, "href"), "/")
		if href != wantPath && href != u.Path {
			continue
		}
		if r.hasElement("addressbook") {
			return true, nil
		}
	}
	return false, nil
}

// propfind performs a PROPFIND depth:1 request and parses the multistatus.
func (c *Client) propfind(ctx context.Context, url, body string) (*multistatus, error) {
	req, err := http.NewRequestWithContext(ctx, "PROPFIND", url, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Depth", "1")
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")

	resp, err := c.transport.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusMultiStatus && resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("carddav: PROPFIND %s returned %s", url, resp.Status)
	}

	return decodeMultistatus(resp.Body)
}

// decodeMultistatus parses a multistatus body. It captures each response's
// raw inner XML so the caller can extract whichever props it needs.
func decodeMultistatus(r io.Reader) (*multistatus, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	ms := &multistatus{}
	if err := xml.Unmarshal(data, ms); err != nil {
		return nil, fmt.Errorf("carddav: parse multistatus: %w", err)
	}
	return ms, nil
}

// hasElement reports whether the response's property XML contains an element
// with the given local name (in any namespace).
func (r *response) hasElement(localName string) bool {
	var root struct {
		Children []xmlElem `xml:",any"`
	}
	if err := xml.Unmarshal([]byte(fmt.Sprintf(responseFragment, r.Raw)), &root); err != nil {
		return false
	}
	return hasElement(root.Children, localName)
}

// SyncToken returns the current sync-token of the address book (used to
// decide whether the next pull can be incremental). Returns "" when the
// server does not support sync tokens.
func (c *Client) SyncToken(ctx context.Context) (string, error) {
	body := `<?xml version="1.0" encoding="utf-8" ?>
<d:propfind xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav">
  <d:prop>
    <d:sync-token/>
  </d:prop>
</d:propfind>`

	ms, err := c.propfind(ctx, c.addressBookURL, body)
	if err != nil {
		return "", err
	}
	for _, r := range ms.Responses {
		if v := extractPropValue(r.Raw, "sync-token"); v != "" {
			return v, nil
		}
	}
	return "", nil
}

// SyncCollection runs a sync-collection REPORT. token == "" requests a full
// (initial) sync. Returns the changed/deleted hrefs plus the new sync-token.
func (c *Client) SyncCollection(ctx context.Context, token string) (*syncReport, error) {
	sc := syncCollection{SyncLevel: "1", SyncToken: token, Prop: []string{"getetag"}}
	body, err := xml.Marshal(sc)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "REPORT", c.addressBookURL, strings.NewReader(xmlHeader(string(body))))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Depth", "1")
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")

	resp, err := c.transport.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusMultiStatus && resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("carddav: sync-collection returned %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	report := &syncReport{}
	if err := xml.Unmarshal(data, report); err != nil {
		return nil, fmt.Errorf("carddav: parse sync report: %w", err)
	}
	for i := range report.Responses {
		report.Responses[i].Href = strings.TrimSpace(report.Responses[i].Href)
	}
	return report, nil
}

// GetVCards fetches a set of vCard resources by href and returns each
// resource's raw vCard text keyed by href.
func (c *Client) GetVCards(ctx context.Context, hrefs []string) (map[string]string, error) {
	out := make(map[string]string, len(hrefs))
	for _, href := range hrefs {
		if href == "" {
			continue
		}
		data, err := c.Get(ctx, href)
		if err != nil {
			return nil, err
		}
		out[href] = string(data)
	}
	return out, nil
}

// Get fetches a single resource by href.
func (c *Client) Get(ctx context.Context, href string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, href, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/vcard, text/x-vcard, text/directory, */*")
	resp, err := c.transport.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("carddav: GET %s returned %s", href, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// PutResult carries the server response to a PUT: the assigned ETag and
// (when returned) the new sync-token.
type PutResult struct {
	ETag string
}

// Put creates or replaces the resource at href with data. When etag is
// non-empty the request is conditional (If-Match); when stale is true the
// request uses If-None-Match: * to fail if the resource already exists.
func (c *Client) Put(ctx context.Context, href string, data []byte, etag string) (*PutResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, href, strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/vcard; charset=utf-8")
	if etag != "" {
		req.Header.Set("If-Match", etag)
	} else {
		req.Header.Set("If-None-Match", "*")
	}

	resp, err := c.transport.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("carddav: PUT %s returned %s", href, resp.Status)
	}

	return &PutResult{ETag: resp.Header.Get("ETag")}, nil
}

// Delete removes the resource at href. Returns (true, nil) when the server
// confirmed deletion, (false, nil) when it was already gone (404), and an
// error otherwise.
func (c *Client) Delete(ctx context.Context, href string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, href, nil)
	if err != nil {
		return false, err
	}
	resp, err := c.transport.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent, http.StatusAccepted:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("carddav: DELETE %s returned %s", href, resp.Status)
	}
}

// xmlHeader prepends an XML declaration to a serialized document.
func xmlHeader(body string) string {
	return `<?xml version="1.0" encoding="utf-8" ?>` + body
}

// extractPropValue pulls the text content of the first element with the given
// local name out of a raw <response> inner-XML fragment. Property elements can
// live in any namespace (DAV:, carddav:, ...), so each element is matched by
// its local name only.
func extractPropValue(raw, localName string) string {
	var root struct {
		Children []xmlElem `xml:",any"`
	}
	if err := xml.Unmarshal([]byte(fmt.Sprintf(responseFragment, raw)), &root); err != nil {
		return ""
	}
	return findText(root.Children, localName)
}

// resolveRelative joins a possibly-relative href onto a base URL.
func resolveRelative(base, href string) string {
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	if strings.HasPrefix(href, "/") {
		// Scheme + host of base + href.
		idx := strings.Index(base, "://")
		if idx < 0 {
			return href
		}
		rest := base[idx+3:]
		slash := strings.IndexByte(rest, '/')
		host := rest
		if slash >= 0 {
			host = rest[:slash]
		}
		return base[:idx+3] + host + href
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(href, "/")
}
