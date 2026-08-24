package i18n

import (
	"context"
	"embed"
	"encoding/json"
	"strings"
)

//go:embed locales/*.json
var localesFS embed.FS

// catalogs holds locale -> key -> translation
var catalogs map[string]map[string]string

func init() {
	catalogs = make(map[string]map[string]string)
	entries, err := localesFS.ReadDir("locales")
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		locale := strings.TrimSuffix(name, ".json")
		data, err := localesFS.ReadFile("locales/" + name)
		if err != nil {
			continue
		}
		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		catalogs[locale] = m
	}
	if _, ok := catalogs["en"]; !ok {
		catalogs["en"] = make(map[string]string)
	}
}

// T returns the translation for key in locale, falling back to en, then to key itself.
func T(locale, key string) string {
	if locale != "" {
		if cat, ok := catalogs[locale]; ok {
			if v, ok := cat[key]; ok {
				return v
			}
		}
	}
	if cat, ok := catalogs["en"]; ok {
		if v, ok := cat[key]; ok {
			return v
		}
	}
	return key
}

// Supported reports whether locale has a catalog.
func Supported(locale string) bool {
	_, ok := catalogs[locale]
	return ok
}

// Locales returns available locale codes.
func Locales() []string {
	out := make([]string, 0, len(catalogs))
	for k := range catalogs {
		out = append(out, k)
	}
	return out
}

type localeContextKey struct{}

// WithLocale returns a context with locale set.
func WithLocale(ctx context.Context, locale string) context.Context {
	return context.WithValue(ctx, localeContextKey{}, locale)
}

// LocaleFromContext extracts locale from context or returns "en".
func LocaleFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(localeContextKey{}).(string); ok && v != "" {
		return v
	}
	return "en"
}

// NormalizeLocale validates against pattern ^[a-z]{2}(-[A-Z]{2})?$ and returns base language code.
// For now we only support 2-letter codes like "en", "de". Region variants map to base if base is supported.
func NormalizeLocale(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// Extract primary tag before - or _
	tag := raw
	if idx := strings.IndexAny(tag, "-_"); idx != -1 {
		tag = tag[:idx]
	}
	tag = strings.ToLower(tag)
	if len(tag) != 2 {
		return ""
	}
	for _, ch := range tag {
		if ch < 'a' || ch > 'z' {
			return ""
		}
	}
	return tag
}

// ResolveLocale implements user > Accept-Language > en.
func ResolveLocale(userLocale string, acceptLanguage string) string {
	if n := NormalizeLocale(userLocale); n != "" && Supported(n) {
		return n
	}
	if acceptLanguage != "" {
		for _, part := range strings.Split(acceptLanguage, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			// strip quality value
			if idx := strings.Index(part, ";"); idx != -1 {
				part = part[:idx]
			}
			part = strings.TrimSpace(part)
			n := NormalizeLocale(part)
			if n != "" && Supported(n) {
				return n
			}
		}
	}
	return "en"
}
