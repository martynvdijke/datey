package web

import (
	"embed"
	"html/template"
)

//go:embed templates/*.html
var templateFS embed.FS

func loadTemplates() (map[string]*template.Template, error) {
	base, err := template.ParseFS(templateFS, "templates/base.html")
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
		"error.html",
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
