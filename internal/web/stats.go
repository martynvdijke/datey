package web

import (
	"net/http"
	"sort"
	"time"

	"github.com/datey/datey/internal/age"
	"github.com/datey/datey/internal/milestone"
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

	h.render(w, r, "stats.html", map[string]any{
		"Title":              "Datey - Stats",
		"AgeBuckets":         ageBuckets,
		"TotalPeople":        totalPeople,
		"BusiestMonths":      busiest,
		"MonthCounts":        monthCounts,
		"EventsPerMonth":     eventsPerMonth,
		"UpcomingMilestones": upcomingMilestones,
		"MissedRecently":     missedRecently,
	})
}
