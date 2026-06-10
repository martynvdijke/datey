package web

import (
	"embed"
	"html/template"
	"math"
)

//go:embed templates/*.html
var templateFS embed.FS

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
}

func loadTemplates() (map[string]*template.Template, error) {
	base, err := template.New("base.html").Funcs(funcMap).ParseFS(templateFS, "templates/base.html")
	if err != nil {
		return nil, err
	}

	pages := []string{
		"dashboard.html",
		"contacts.html",
		"contact_detail.html",
		"contact_form.html",
		"event_form.html",
		"settings.html",
		"logs.html",
		"error.html",
		"login.html",
		"setup.html",
		"users.html",
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

