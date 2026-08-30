package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"time"

	"github.com/datey/datey/internal/age"
	"github.com/datey/datey/internal/i18n"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

var funcMap = template.FuncMap{
	"now": time.Now,
	"seq": func(start, end int) []int {
		var out []int
		for i := start; i <= end; i++ {
			out = append(out, i)
		}
		return out
	},
	"add":      func(a, b int) int { return a + b },
	"sub":      func(a, b int) int { return a - b },
	"subtract": func(a, b int) int { return a - b },
	"div":      func(a, b int) int { return int(math.Ceil(float64(a) / float64(b))) },
	"divFloat": func(a *int, b float64) float64 {
		if a == nil {
			return 0
		}
		return float64(*a) / b
	},
	"iter": func(n int) []int {
		r := make([]int, n)
		for i := range r {
			r[i] = i
		}
		return r
	},
	"list": func(vals ...string) []string { return vals },
	"inList": func(list any, item string) bool {
		s, ok := list.([]string)
		if !ok {
			return false
		}
		for _, v := range s {
			if v == item {
				return true
			}
		}
		return false
	},
	"dict": func(values ...any) map[string]any {
		if len(values)%2 != 0 {
			panic("dict: odd number of arguments")
		}
		m := make(map[string]any, len(values)/2)
		for i := 0; i < len(values); i += 2 {
			key, ok := values[i].(string)
			if !ok {
				panic("dict: non-string key")
			}
			m[key] = values[i+1]
		}
		return m
	},
	// birthdayAge derives a person's age from their birthday event date.
	// HasAge is false when no usable birth year exists; templates then omit
	// the age text (see internal/age).
	"birthdayAge": age.InfoFor,
	"jsonEncode": func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(b)
	},
	"ordinal": func(n int) string {
		if n%100 >= 11 && n%100 <= 13 {
			return fmt.Sprintf("%dth", n)
		}
		switch n % 10 {
		case 1:
			return fmt.Sprintf("%dst", n)
		case 2:
			return fmt.Sprintf("%dnd", n)
		case 3:
			return fmt.Sprintf("%drd", n)
		}
		return fmt.Sprintf("%dth", n)
	},
	"T": func(key string) string {
		return i18n.T("en", key)
	},
}

func loadTemplates() (map[string]*template.Template, error) {
	base, err := template.New("base.html").Funcs(funcMap).ParseFS(templateFS, "templates/base.html")
	if err != nil {
		return nil, err
	}

	pages := []string{
		"dashboard.html",
		"stats.html",
		"people.html",
		"person_detail.html",
		"person_form.html",
		"groups.html",
		"group_detail.html",
		"event_form.html",
		"calendar.html",
		"settings.html",
		"error.html",
		"login.html",
		"setup.html",
		"users.html",
		"api_tokens.html",
		"forgot_password.html",
		"reset_password.html",
		"recurring_rule_form.html",
		"recurring_rule_list.html",
		"audit.html",
	}

	templates := make(map[string]*template.Template, len(pages))
	for _, page := range pages {
		t, err := template.Must(base.Clone()).ParseFS(templateFS, "templates/"+page)
		if err != nil {
			return nil, err
		}
		templates[page] = t
	}

	// Partials are standalone fragments rendered directly into HTMX responses
	// (no base layout).
	partials := []string{
		"immich_sync_result.html",
	}
	for _, partial := range partials {
		t, err := template.New(partial).Funcs(funcMap).ParseFS(templateFS, "templates/"+partial)
		if err != nil {
			return nil, err
		}
		templates[partial] = t
	}

	return templates, nil
}
