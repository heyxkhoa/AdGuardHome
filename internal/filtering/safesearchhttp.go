package filtering

import (
	"encoding/json"
	"net/http"

	"github.com/AdguardTeam/AdGuardHome/internal/aghhttp"
)

// handleSafeSearchEnable is the handler for POST /control/safesearch/enable
// HTTP API.
//
// Deprecated: Use handleSafeSearchSettings.
func (d *DNSFilter) handleSafeSearchEnable(w http.ResponseWriter, r *http.Request) {
	setProtectedBool(&d.confLock, &d.Config.SafeSearchConf.Enabled, true)
	d.Config.ConfigModified()
}

// handleSafeSearchDisable is the handler for POST /control/safesearch/disable
// HTTP API.
//
// Deprecated: Use handleSafeSearchSettings.
func (d *DNSFilter) handleSafeSearchDisable(w http.ResponseWriter, r *http.Request) {
	setProtectedBool(&d.confLock, &d.Config.SafeSearchConf.Enabled, false)
	d.Config.ConfigModified()
}

// handleSafeSearchStatus is the handler for GET /control/safesearch/status
// HTTP API.
func (d *DNSFilter) handleSafeSearchStatus(w http.ResponseWriter, r *http.Request) {
	d.confLock.RLock()
	resp := d.Config.SafeSearchConf
	d.confLock.RUnlock()

	_ = aghhttp.WriteJSONResponse(w, r, resp)
}

// handleSafeSearchSettings is the handler for PUT /control/safesearch/settings
// HTTP API.
func (d *DNSFilter) handleSafeSearchSettings(w http.ResponseWriter, r *http.Request) {
	req := &SafeSearchConfig{}
	err := json.NewDecoder(r.Body).Decode(req)
	if err != nil {
		aghhttp.Error(r, w, http.StatusBadRequest, "reading req: %s", err)

		return
	}

	d.confLock.Lock()
	d.Config.SafeSearchConf = *req
	d.confLock.Unlock()

	d.Config.ConfigModified()

	aghhttp.OK(w)
}
