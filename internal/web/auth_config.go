package web

import (
	"net/http"
	"strings"
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

// loginData adds demo-account details to a login template's data when the
// demo account is enabled. Keeping this in one helper avoids repeating the
// fields across the initial render and every error branch.
func (h *Handler) loginData(data map[string]any) map[string]any {
	if !h.demoLoginEnabled() {
		return data
	}
	data["DemoEnabled"] = true
	data["DemoHandle"] = strings.ToLower(strings.TrimSpace(h.cfg.DemoUserHandle))
	data["DemoPassword"] = h.cfg.DemoUserPassword
	return data
}
