package web

import (
	"context"
	"log/slog"
)

// personOption is a simplified person representation for form dropdowns.
type personOption struct {
	ID   int
	Name string
}

// personOptions returns all people as dropdown options.
func (h *Handler) personOptions(ctx context.Context) []personOption {
	people, err := h.people.List(ctx)
	if err != nil {
		slog.Error("list people for form options", "error", err)
		return nil
	}
	opts := make([]personOption, 0, len(people))
	for _, p := range people {
		opts = append(opts, personOption{ID: p.ID, Name: p.Name})
	}
	return opts
}
