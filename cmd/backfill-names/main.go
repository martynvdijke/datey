// Command backfill-names populates the structured name columns
// (first_name/middle_name/last_name) for existing persons that only have the
// legacy display `name`. The split is heuristic: first token → first name,
// remainder → last name (see internal/person.SplitDisplayName).
//
// The script is idempotent: rows that already have any structured part set
// are skipped, so it can be re-run safely. Rows are updated one by one and
// the display `name` column is never modified.
//
// Usage:
//
//	go run ./cmd/backfill-names            # uses config.Load() for DATA_DIR
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"

	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/db"
	"github.com/datey/datey/internal/person"
	"github.com/datey/datey/internal/repository"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "report what would change without writing")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "error", err)
		return
	}
	client, err := db.Init(cfg)
	if err != nil {
		slog.Error("open database", "path", cfg.DataDir+"/datey.db", "error", err)
		return
	}
	defer func() {
		if cerr := client.Close(); cerr != nil {
			slog.Warn("close database", "error", cerr)
		}
	}()

	ctx := context.Background()
	people := repository.NewPersonRepository(client)
	persons, err := people.List(ctx)
	if err != nil {
		slog.Error("list persons", "error", err)
		return
	}

	updated, skipped := 0, 0
	for _, p := range persons {
		if p.FirstName != nil || p.MiddleName != nil || p.LastName != nil {
			skipped++
			continue
		}
		first, _, last := person.SplitDisplayName(p.Name)
		if *dryRun {
			fmt.Printf("would update #%d %q -> first=%q last=%q\n", p.ID, p.Name, first, last)
			updated++
			continue
		}
		if _, err := people.UpdateStructured(ctx, p.ID, p.Name, first, "", last, p.Notes, ""); err != nil {
			slog.Error("backfill person", "id", p.ID, "name", p.Name, "error", err)
			return
		}
		updated++
	}

	fmt.Printf("backfill complete: %d updated, %d skipped (already structured)\n", updated, skipped)
}
