package web

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/datey/datey/internal/recurring"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) listRecurringRules(w http.ResponseWriter, r *http.Request) {
	rules, err := h.recurringRules.List(r.Context())
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}
	h.render(w, r, "recurring_rule_list.html", map[string]any{
		"Title": "Datey - Recurring Rules",
		"Rules": rules,
	})
}

func (h *Handler) newRecurringRuleForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "recurring_rule_form.html", map[string]any{
		"Title": "Datey - New Recurring Rule",
	})
}

func (h *Handler) createRecurringRule(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.FormValue("name")
	patternType := r.FormValue("pattern_type")
	monthStr := r.FormValue("month")
	nthStr := r.FormValue("nth")
	weekdayStr := r.FormValue("weekday")
	dayStr := r.FormValue("day")

	errors := make(map[string]string)
	if name == "" {
		errors["name"] = "Name is required"
	}
	if patternType == "" {
		errors["pattern_type"] = "Type is required"
	}

	month, nth, weekday, day := 0, 0, 0, 0
	switch patternType {
	case recurring.NthWeekdayRule:
		if monthStr == "" {
			errors["month"] = "Month is required"
		} else {
			v, err := strconv.Atoi(monthStr)
			if err != nil || v < 1 || v > 12 {
				errors["month"] = "Month must be 1-12"
			} else {
				month = v
			}
		}
		if nthStr == "" {
			errors["nth"] = "Ordinal is required"
		} else {
			v, err := strconv.Atoi(nthStr)
			if err != nil || v < 1 || v > 5 {
				errors["nth"] = "Ordinal must be 1-5"
			} else {
				nth = v
			}
		}
		if weekdayStr == "" {
			errors["weekday"] = "Weekday is required"
		} else {
			v, err := strconv.Atoi(weekdayStr)
			if err != nil || v < 0 || v > 6 {
				errors["weekday"] = "Weekday must be 0-6"
			} else {
				weekday = v
			}
		}
	case "fixed":
		if monthStr == "" {
			errors["month"] = "Month is required"
		} else {
			v, _ := strconv.Atoi(monthStr)
			month = v
		}
		if dayStr == "" {
			errors["day"] = "Day is required"
		} else {
			v, _ := strconv.Atoi(dayStr)
			day = v
		}
	}

	if len(errors) > 0 {
		h.render(w, r, "recurring_rule_form.html", map[string]any{
			"Title":  "Datey - New Recurring Rule",
			"Errors": errors,
			"FormData": map[string]string{
				"Name":        name,
				"PatternType": patternType,
				"Month":       monthStr,
				"Nth":         nthStr,
				"Weekday":     weekdayStr,
				"Day":         dayStr,
			},
		})
		return
	}

	_, err := h.recurringRules.Create(r.Context(), name, patternType, nth, weekday, month, day)
	if err != nil {
		errors["name"] = "Failed to create rule"
		h.render(w, r, "recurring_rule_form.html", map[string]any{
			"Title":  "Datey - New Recurring Rule",
			"Errors": errors,
			"FormData": map[string]string{
				"Name":        name,
				"PatternType": patternType,
				"Month":       monthStr,
				"Nth":         nthStr,
				"Weekday":     weekdayStr,
				"Day":         dayStr,
			},
		})
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/recurring-rules?success=Rule+%s+created", name), http.StatusSeeOther)
}

func (h *Handler) deleteRecurringRule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := h.recurringRules.Delete(r.Context(), id); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	toastHeader(w, "Rule deleted", "success")
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}
