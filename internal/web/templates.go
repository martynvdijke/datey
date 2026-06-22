package web

import (
	"embed"
	"html/template"
	"math"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

var funcMap = template.FuncMap{
	"add": func(a, b int) int { return a + b },
	"sub": func(a, b int) int { return a - b },
	"div": func(a, b int) int { return int(math.Ceil(float64(a) / float64(b))) },
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
}

func loadTemplates() (map[string]*template.Template, error) {
	base, err := template.New("base.html").Funcs(funcMap).ParseFS(templateFS, "templates/base.html")
	if err != nil {
		return nil, err
	}

	pages := []string{
		"dashboard.html",
		"people.html",
		"person_detail.html",
		"person_form.html",
		"groups.html",
		"event_form.html",
		"calendar.html",
		"settings.html",
		"error.html",
		"login.html",
		"setup.html",
		"users.html",
		"notifications.html",
		"notification_form.html",
	}

	templates := make(map[string]*template.Template, len(pages))
	for _, page := range pages {
		t, err := template.Must(base.Clone()).ParseFS(templateFS, "templates/"+page)
		if err != nil {
			return nil, err
		}
		templates[page] = t
	}

	return templates, nil
}

