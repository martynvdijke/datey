package db

import (
	"context"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/recurringrule"
	"github.com/datey/datey/internal/config"

	_ "github.com/mattn/go-sqlite3"
)

func Init(cfg *config.Config) (*ent.Client, error) {
	dbPath := cfg.DataDir + "/datey.db"
	client, err := ent.Open("sqlite3", dbPath+"?_journal_mode=WAL&_timeout=5000&_fk=1")
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	if err := client.Schema.Create(ctx); err != nil {
		return nil, err
	}

	seedBuiltInRules(ctx, client)

	return client, nil
}

func seedBuiltInRules(ctx context.Context, client *ent.Client) {
	rules := []struct {
		Name        string
		PatternType string
		Nth         int
		Weekday     int
		Month       int
		Day         int
	}{
		{Name: "Mother's Day", PatternType: "nth_weekday", Nth: 2, Weekday: 0, Month: 5},
		{Name: "Father's Day", PatternType: "nth_weekday", Nth: 3, Weekday: 0, Month: 6},
		{Name: "New Year's Day", PatternType: "fixed", Month: 1, Day: 1},
	}

	for _, r := range rules {
		exists, _ := client.RecurringRule.Query().Where(
			recurringrule.NameEQ(r.Name),
		).Exist(ctx)
		if exists {
			continue
		}

		q := client.RecurringRule.Create().
			SetName(r.Name).
			SetPatternType(r.PatternType).
			SetNth(r.Nth).
			SetWeekday(r.Weekday).
			SetMonth(r.Month).
			SetDay(r.Day).
			SetCreatedAt(time.Now())

		q.Save(ctx)
	}
}
