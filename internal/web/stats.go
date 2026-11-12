package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/datey/datey/internal/age"
	"github.com/datey/datey/internal/milestone"
	"github.com/datey/datey/internal/repository"
	"github.com/datey/datey/internal/tzutil"
)

type ageBucket struct {
	Label string
	Count int
	Pct   int
}

type monthCount struct {
	Month int
	Label string
	Count int
	Pct   int
}

type milestoneView struct {
	Name  string
	Type  string
	Date  string
	Age   int
	Label string
}

type missedView struct {
	Name string
	Type string
	Date string
}

// giftStatusTotal is one row of the gift-spend breakdown. Amounts stay in
// integer cents until rendering; Total is the formatted display string.
type giftStatusTotal struct {
	Status string
	Count  int
	Cents  int
	Total  string
}

// giftSpender is one row of the per-person spend table.
type giftSpender struct {
	Name  string
	Count int
	Cents int
	Total string
}

// giftMonthSpend is one row of the spend-by-month bar chart. Undated marks the
// explicit bucket for priced purchases that carry no lifecycle timestamp.
type giftMonthSpend struct {
	Label   string
	Undated bool
	Count   int
	Cents   int
	Total   string
	Pct     int
}

// currencySymbol is the single place the gift-spend currency is chosen. The
// app has no currency setting yet and renders gift prices with "$" on the
// person page, so the stats summary matches that.
const currencySymbol = "$"

// formatCents renders an integer amount of cents as a money string
// ("$1,234.56"). Integer arithmetic guarantees two exact decimals, so no
// floating-point rounding can creep into the summary.
func formatCents(cents int) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	whole := strconv.Itoa(cents / 100)
	var grouped strings.Builder
	for i := 0; i < len(whole); i++ {
		if i > 0 && (len(whole)-i)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteByte(whole[i])
	}
	return fmt.Sprintf("%s%s%s.%02d", sign, currencySymbol, grouped.String(), cents%100)
}

func (h *Handler) statsPage(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	today := todayStartUTC()

	// People for age distribution
	people, _ := h.people.List(r.Context())
	events, _ := h.events.List(r.Context())
	// occurrences for histograms / milestones
	upcoming60, _ := h.events.ListUpcomingOccurrences(r.Context(), today, today.AddDate(0, 0, 60))
	missedFrom := today.AddDate(0, 0, -30)
	missedTo := today.AddDate(0, 0, -1)
	missedOcc, _ := h.events.ListUpcomingOccurrences(r.Context(), missedFrom, missedTo)

	// Age buckets
	bucketLabels := []string{"<18", "18-30", "30-50", "50-70", "70+", "unknown"}
	bucketCounts := map[string]int{}
	for _, l := range bucketLabels {
		bucketCounts[l] = 0
	}
	// Map personID -> birthday date
	// Use events list filtered to birthday
	birthdayByPerson := map[int]time.Time{}
	birthdayByContact := map[int]time.Time{}
	for _, e := range events {
		if e.Type != "birthday" {
			continue
		}
		if p := e.Edges.Person; p != nil {
			// keep first
			if _, ok := birthdayByPerson[p.ID]; !ok {
				birthdayByPerson[p.ID] = e.Date
			}
		} else if c := e.Edges.Contact; c != nil {
			if _, ok := birthdayByContact[c.ID]; !ok {
				birthdayByContact[c.ID] = e.Date
			}
		}
	}
	// For each person, determine bucket
	for _, p := range people {
		bd, ok := birthdayByPerson[p.ID]
		if !ok {
			bucketCounts["unknown"]++
			continue
		}
		current, hasAge := age.AgeAt(bd, now)
		if !hasAge {
			bucketCounts["unknown"]++
			continue
		}
		switch {
		case current < 18:
			bucketCounts["<18"]++
		case current <= 30:
			bucketCounts["18-30"]++
		case current <= 50:
			bucketCounts["30-50"]++
		case current <= 70:
			bucketCounts["50-70"]++
		default:
			bucketCounts["70+"]++
		}
	}
	totalPeople := len(people)
	var ageBuckets []ageBucket
	maxAgeCount := 0
	for _, l := range bucketLabels {
		if bucketCounts[l] > maxAgeCount {
			maxAgeCount = bucketCounts[l]
		}
	}
	for _, l := range bucketLabels {
		c := bucketCounts[l]
		pct := 0
		if maxAgeCount > 0 {
			pct = c * 100 / maxAgeCount
		}
		ageBuckets = append(ageBuckets, ageBucket{Label: l, Count: c, Pct: pct})
	}

	// Busiest birthday months
	monthCounts := make([]monthCount, 12)
	monthNames := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	for i := range monthCounts {
		monthCounts[i] = monthCount{Month: i + 1, Label: monthNames[i]}
	}
	for _, e := range events {
		if e.Type != "birthday" {
			continue
		}
		m := int(e.Date.Month())
		if m >= 1 && m <= 12 {
			monthCounts[m-1].Count++
		}
	}
	// sort copy for busiest
	busiest := make([]monthCount, len(monthCounts))
	copy(busiest, monthCounts)
	sort.Slice(busiest, func(i, j int) bool {
		if busiest[i].Count == busiest[j].Count {
			return busiest[i].Month < busiest[j].Month
		}
		return busiest[i].Count > busiest[j].Count
	})
	maxMonth := 0
	for _, mc := range monthCounts {
		if mc.Count > maxMonth {
			maxMonth = mc.Count
		}
	}
	for i := range monthCounts {
		if maxMonth > 0 {
			monthCounts[i].Pct = monthCounts[i].Count * 100 / maxMonth
		}
	}
	maxBusiest := 0
	if len(busiest) > 0 {
		maxBusiest = busiest[0].Count
	}
	for i := range busiest {
		if maxBusiest > 0 {
			busiest[i].Pct = busiest[i].Count * 100 / maxBusiest
		}
	}

	// Events per month histogram: count all events per month (by stored month)
	eventsPerMonth := make([]monthCount, 12)
	for i := range eventsPerMonth {
		eventsPerMonth[i] = monthCount{Month: i + 1, Label: monthNames[i]}
	}
	for _, e := range events {
		m := int(e.Date.Month())
		if m >= 1 && m <= 12 {
			eventsPerMonth[m-1].Count++
		}
	}
	maxEPM := 0
	for _, mc := range eventsPerMonth {
		if mc.Count > maxEPM {
			maxEPM = mc.Count
		}
	}
	for i := range eventsPerMonth {
		if maxEPM > 0 {
			eventsPerMonth[i].Pct = eventsPerMonth[i].Count * 100 / maxEPM
		}
	}

	// Upcoming milestones (next 60 days)
	var upcomingMilestones []milestoneView
	for _, occ := range upcoming60 {
		if ok, label := milestone.IsMilestone(occ.Event.Type, occ.Event.Date, occ.Date); ok {
			nAge, _ := age.NextAge(occ.Event.Date, occ.Date)
			// For anniversary etc, keep label; age not relevant but show
			upcomingMilestones = append(upcomingMilestones, milestoneView{
				Name:  eventOwnerName(occ.Event),
				Type:  occ.Event.Type,
				Date:  shortDate(h.cfg.DateVariant, occ.Date),
				Age:   nAge,
				Label: label,
			})
		}
	}

	// Missed recently (last 30 days)
	var missedRecently []missedView
	for _, occ := range missedOcc {
		missedRecently = append(missedRecently, missedView{
			Name: eventOwnerName(occ.Event),
			Type: occ.Event.Type,
			Date: shortDate(h.cfg.DateVariant, occ.Date),
		})
	}

	// Gift spend. One ListAll (ideas + eager-loaded persons) feeds both the
	// all-time totals and the trailing 12-month breakdown; bucketing happens in
	// the repository so no extra query or raw SQL is needed. GivenAt is the
	// spend date when present, else PurchasedAt; items with neither land in the
	// explicit "undated" bucket.
	giftIdeas, giftErr := h.giftIdeas.ListAll(r.Context())
	if giftErr != nil {
		slog.Error("stats: list gift ideas", "error", giftErr)
	}
	giftStatusOrder := []string{"idea", "purchased", "given", "archived"}
	giftCountByStatus := map[string]int{}
	giftCentsByStatus := map[string]int{}
	giftTotalCents := 0
	giftTotalCount := len(giftIdeas)
	giftPricedCount := 0
	spendByPerson := map[int]*giftSpender{}
	for _, g := range giftIdeas {
		giftCountByStatus[string(g.Status)]++
		if g.PriceCents == nil {
			continue
		}
		cents := *g.PriceCents
		if cents < 0 {
			cents = 0
		}
		giftPricedCount++
		giftCentsByStatus[string(g.Status)] += cents
		giftTotalCents += cents
		if p := g.Edges.Person; p != nil {
			row, ok := spendByPerson[p.ID]
			if !ok {
				row = &giftSpender{Name: p.Name}
				spendByPerson[p.ID] = row
			}
			row.Count++
			row.Cents += cents
		}
	}
	var giftByStatus []giftStatusTotal
	for _, status := range giftStatusOrder {
		if giftCountByStatus[status] == 0 {
			continue
		}
		giftByStatus = append(giftByStatus, giftStatusTotal{
			Status: status,
			Count:  giftCountByStatus[status],
			Cents:  giftCentsByStatus[status],
			Total:  formatCents(giftCentsByStatus[status]),
		})
	}
	topSpenders := make([]giftSpender, 0, len(spendByPerson))
	for _, row := range spendByPerson {
		row.Total = formatCents(row.Cents)
		topSpenders = append(topSpenders, *row)
	}
	sort.Slice(topSpenders, func(i, j int) bool {
		if topSpenders[i].Cents == topSpenders[j].Cents {
			return topSpenders[i].Name < topSpenders[j].Name
		}
		return topSpenders[i].Cents > topSpenders[j].Cents
	})
	const topSpenderLimit = 5
	if len(topSpenders) > topSpenderLimit {
		topSpenders = topSpenders[:topSpenderLimit]
	}

	// 12-month spend bars. Percentages are relative to the largest bucket so
	// the widest bar fills the track; a non-zero amount never rounds to a
	// zero-width bar.
	giftMonthlyBuckets := repository.MonthlySpend(giftIdeas, time.Now(), 12, tzutil.Default())
	giftMonthly := make([]giftMonthSpend, 0, len(giftMonthlyBuckets))
	maxMonthCents := 0
	for _, b := range giftMonthlyBuckets {
		if b.Cents > maxMonthCents {
			maxMonthCents = b.Cents
		}
	}
	for _, b := range giftMonthlyBuckets {
		row := giftMonthSpend{
			Undated: b.Undated,
			Count:   b.Count,
			Cents:   b.Cents,
			Total:   formatCents(b.Cents),
		}
		if !b.Undated {
			row.Label = b.Month.Format("Jan 06")
		}
		if maxMonthCents > 0 {
			row.Pct = (b.Cents*100 + maxMonthCents - 1) / maxMonthCents
		}
		giftMonthly = append(giftMonthly, row)
	}

	h.render(w, r, "stats.html", map[string]any{
		"Title":              "Datey - Stats",
		"AgeBuckets":         ageBuckets,
		"TotalPeople":        totalPeople,
		"BusiestMonths":      busiest,
		"MonthCounts":        monthCounts,
		"EventsPerMonth":     eventsPerMonth,
		"UpcomingMilestones": upcomingMilestones,
		"MissedRecently":     missedRecently,
		"GiftByStatus":       giftByStatus,
		"GiftTopSpenders":    topSpenders,
		"GiftMonthlySpend":   giftMonthly,
		"GiftTotal":          formatCents(giftTotalCents),
		"GiftTotalCount":     giftTotalCount,
		"GiftPricedCount":    giftPricedCount,
	})
}
