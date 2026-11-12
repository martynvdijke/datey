package web

import (
	"net/http"
	"strings"

	"github.com/datey/datey/handlers"
)

type demoAuthConfig struct {
	Enabled  bool   `json:"enabled"`
	Handle   string `json:"handle,omitempty"`
	Password string `json:"password,omitempty"`
}

type authConfigResponse struct {
	Demo demoAuthConfig `json:"demo"`
}

// authConfig exposes public authentication hints used by the login screen and
// native clients. No authentication is required.
// demoLoginEnabled reports whether demo credentials can actually be
// advertised. An enabled flag without a usable handle or password would render
// a broken login form, so both are required.
func (h *Handler) demoLoginEnabled() bool {
	return h.cfg != nil &&
		h.cfg.DemoUserEnabled &&
		strings.TrimSpace(h.cfg.DemoUserHandle) != "" &&
		h.cfg.DemoUserPassword != ""
}

func (h *Handler) authConfig(w http.ResponseWriter, r *http.Request) {
	demoCfg := demoAuthConfig{}
	if h.demoLoginEnabled() {
		demoCfg.Enabled = true
		demoCfg.Handle = strings.ToLower(strings.TrimSpace(h.cfg.DemoUserHandle))
		demoCfg.Password = h.cfg.DemoUserPassword
	}
	writeAPIJSON(w, http.StatusOK, authConfigResponse{Demo: demoCfg})
}

// authPageData adds the values every unauthenticated auth surface shares: the
// running application version for the quiet card footer, plus the demo
// credentials when they apply. handlers.Version is the single source of truth
// (main.go sets it from the build version). Keeping it in one helper avoids
// repeating fields across the initial render and every error branch.
func (h *Handler) authPageData(data map[string]any) map[string]any {
	data["Version"] = handlers.Version
	if !h.demoLoginEnabled() {
		return data
	}
	data["DemoEnabled"] = true
	data["DemoHandle"] = strings.ToLower(strings.TrimSpace(h.cfg.DemoUserHandle))
	data["DemoPassword"] = h.cfg.DemoUserPassword
	return data
}

// renderAuth renders a public auth page (login, setup, password reset) with the
// shared auth data. It exists so those pages cannot drift apart: every one of
// them shows the same version footer.
func (h *Handler) renderAuth(w http.ResponseWriter, r *http.Request, page string, data map[string]any) {
	h.render(w, r, page, h.authPageData(data))
}
