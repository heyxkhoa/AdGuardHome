// HTTP request handlers for accessing statistics data and configuration settings

package stats

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/AdguardTeam/AdGuardHome/internal/aghalg"
	"github.com/AdguardTeam/AdGuardHome/internal/aghhttp"
	"github.com/AdguardTeam/AdGuardHome/internal/aghnet"
	"github.com/AdguardTeam/golibs/log"
	"github.com/AdguardTeam/golibs/timeutil"
)

// topAddrs is an alias for the types of the TopFoo fields of statsResponse.
// The key is either a client's address or a requested address.
type topAddrs = map[string]uint64

// StatsResp is a response to the GET /control/stats.
type StatsResp struct {
	TimeUnits string `json:"time_units"`

	TopQueried []topAddrs `json:"top_queried_domains"`
	TopClients []topAddrs `json:"top_clients"`
	TopBlocked []topAddrs `json:"top_blocked_domains"`

	DNSQueries []uint64 `json:"dns_queries"`

	BlockedFiltering     []uint64 `json:"blocked_filtering"`
	ReplacedSafebrowsing []uint64 `json:"replaced_safebrowsing"`
	ReplacedParental     []uint64 `json:"replaced_parental"`

	NumDNSQueries           uint64 `json:"num_dns_queries"`
	NumBlockedFiltering     uint64 `json:"num_blocked_filtering"`
	NumReplacedSafebrowsing uint64 `json:"num_replaced_safebrowsing"`
	NumReplacedSafesearch   uint64 `json:"num_replaced_safesearch"`
	NumReplacedParental     uint64 `json:"num_replaced_parental"`

	AvgProcessingTime float64 `json:"avg_processing_time"`
}

// handleStats handles requests to the GET /control/stats endpoint.
func (s *StatsCtx) handleStats(w http.ResponseWriter, r *http.Request) {
	s.lock.Lock()
	defer s.lock.Unlock()

	start := time.Now()
	resp, ok := s.getData(s.limitHours)
	log.Debug("stats: prepared data in %v", time.Since(start))

	if !ok {
		// Don't bring the message to the lower case since it's a part of UI
		// text for the moment.
		aghhttp.Error(r, w, http.StatusInternalServerError, "Couldn't get statistics data")

		return
	}

	_ = aghhttp.WriteJSONResponse(w, r, resp)
}

// configResp is the response to the GET /control/stats_info.
type configResp struct {
	IntervalDays uint32 `json:"interval"`
}

// configRespV2 is the response to the GET /control/stats_info.
type configRespV2 struct {
	// Enabled shows if statistics are enabled.  It is an [aghalg.NullBool]
	// to be able to tell when it's set without using pointers.
	Enabled aghalg.NullBool `json:"enabled"`

	// Interval is the statistics rotation interval.
	Interval float64 `json:"interval"`

	// Ignored is the list of host names, which should not be counted.
	Ignored []string `json:"ignored,omitempty"`
}

// handleStatsInfo handles requests to the GET /control/stats_info endpoint.
func (s *StatsCtx) handleStatsInfo(w http.ResponseWriter, r *http.Request) {
	s.lock.Lock()
	defer s.lock.Unlock()

	resp := configResp{IntervalDays: s.limitHours / 24}
	if !s.enabled {
		resp.IntervalDays = 0
	}
	_ = aghhttp.WriteJSONResponse(w, r, resp)
}

// handleStatsInfoV2 handles requests to the GET /control/stats_info endpoint.
func (s *StatsCtx) handleStatsInfoV2(w http.ResponseWriter, r *http.Request) {
	s.lock.Lock()
	defer s.lock.Unlock()

	resp := configRespV2{
		Enabled: aghalg.BoolToNullBool(s.enabled),
		// 1 hour = 3,600,600 ms
		Interval: float64(s.limitHours) * 3_600_000,
		Ignored:  s.ignored.Values(),
	}
	_ = aghhttp.WriteJSONResponse(w, r, resp)
}

// handleStatsConfig handles requests to the POST /v2/control/stats_config
// endpoint.
func (s *StatsCtx) handleStatsConfig(w http.ResponseWriter, r *http.Request) {
	reqData := configResp{}
	err := json.NewDecoder(r.Body).Decode(&reqData)
	if err != nil {
		aghhttp.Error(r, w, http.StatusBadRequest, "json decode: %s", err)
		s.lock.Unlock()

		return
	}

	if !checkInterval(reqData.IntervalDays) {
		aghhttp.Error(r, w, http.StatusBadRequest, "Unsupported interval")
		s.lock.Unlock()

		return
	}

	defer s.configModified()

	s.lock.Lock()
	defer s.lock.Unlock()

	s.setLimit(int(reqData.IntervalDays))

	s.configModified()
}

// handleStatsConfigV2 handles requests to the POST /v2/control/stats_config
// endpoint.
func (s *StatsCtx) handleStatsConfigV2(w http.ResponseWriter, r *http.Request) {
	reqData := configRespV2{}
	err := json.NewDecoder(r.Body).Decode(&reqData)
	if err != nil {
		aghhttp.Error(r, w, http.StatusBadRequest, "json decode: %s", err)

		return
	}

	ivl := time.Duration(float64(time.Millisecond) * reqData.Interval)
	days := ivl.Hours() / 24

	if ivl < time.Hour || ivl > timeutil.Day*366 || !checkInterval(uint32(days)) {
		aghhttp.Error(r, w, http.StatusBadRequest, "unsupported interval")

		return
	}

	defer s.configModified()

	s.lock.Lock()
	defer s.lock.Unlock()

	s.setLimit(int(days))

	if len(reqData.Ignored) > 0 {
		set, serr := aghnet.NewDomainNameSet(reqData.Ignored)
		if serr != nil {
			aghhttp.Error(r, w, http.StatusBadRequest, "ignored: %s", serr)

			return
		} else {
			s.ignored = set
		}
	}
}

// handleStatsReset handles requests to the POST /control/stats_reset endpoint.
func (s *StatsCtx) handleStatsReset(w http.ResponseWriter, r *http.Request) {
	err := s.clear()
	if err != nil {
		aghhttp.Error(r, w, http.StatusInternalServerError, "stats: %s", err)
	}
}

// initWeb registers the handlers for web endpoints of statistics module.
func (s *StatsCtx) initWeb() {
	if s.httpRegister == nil {
		return
	}

	s.httpRegister(http.MethodGet, "/control/stats", s.handleStats)
	s.httpRegister(http.MethodPost, "/control/stats_reset", s.handleStatsReset)
	s.httpRegister(http.MethodPost, "/control/stats_config", s.handleStatsConfig)
	s.httpRegister(http.MethodGet, "/control/stats_info", s.handleStatsInfo)

	// API v2.
	s.httpRegister(http.MethodGet, "/control/stats/config", s.handleStatsInfoV2)
	s.httpRegister(http.MethodPut, "/control/stats/config/update", s.handleStatsConfigV2)
}
