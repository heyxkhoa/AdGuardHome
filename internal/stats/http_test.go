package stats

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/AdguardTeam/AdGuardHome/internal/aghalg"
	"github.com/AdguardTeam/golibs/testutil"
	"github.com/AdguardTeam/golibs/timeutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleStatsConfig(t *testing.T) {
	var (
		halfHour = time.Hour / 2
		hour     = time.Hour
		year     = timeutil.Day * 365
	)

	testCases := []struct {
		name     string
		body     getConfigResp
		wantCode int
		wantErr  string
	}{{
		name: "set_ivl_1_hour",
		body: getConfigResp{
			Enabled:  aghalg.NBTrue,
			Interval: float64(hour.Milliseconds()),
			Ignored:  nil,
		},
		wantCode: http.StatusOK,
		wantErr:  "",
	}, {
		name: "small_interval",
		body: getConfigResp{
			Enabled:  aghalg.NBTrue,
			Interval: float64(halfHour.Milliseconds()),
			Ignored:  []string{},
		},
		wantCode: http.StatusUnprocessableEntity,
		wantErr:  "unsupported interval\n",
	}, {
		name: "big_interval",
		body: getConfigResp{
			Enabled:  aghalg.NBTrue,
			Interval: float64(year.Milliseconds() + hour.Milliseconds()),
			Ignored:  []string{},
		},
		wantCode: http.StatusUnprocessableEntity,
		wantErr:  "unsupported interval\n",
	}, {
		name: "set_ignored_ivl_1_year",
		body: getConfigResp{
			Enabled:  aghalg.NBTrue,
			Interval: float64(year.Milliseconds()),
			Ignored: []string{
				"ignor.ed",
			},
		},
		wantCode: http.StatusOK,
		wantErr:  "",
	}, {
		name: "ignored_duplicate",
		body: getConfigResp{
			Enabled:  aghalg.NBTrue,
			Interval: float64(hour.Milliseconds()),
			Ignored: []string{
				"ignor.ed",
				"ignor.ed",
			},
		},
		wantCode: http.StatusUnprocessableEntity,
		wantErr:  "ignored: duplicate host name \"ignor.ed\"\n",
	}, {
		name: "ignored_empty",
		body: getConfigResp{
			Enabled:  aghalg.NBTrue,
			Interval: float64(hour.Milliseconds()),
			Ignored: []string{
				"",
			},
		},
		wantCode: http.StatusUnprocessableEntity,
		wantErr:  "ignored: host name is empty\n",
	}, {
		name: "enabled_is_null",
		body: getConfigResp{
			Enabled:  aghalg.NBNull,
			Interval: float64(hour.Milliseconds()),
			Ignored:  []string{},
		},
		wantCode: http.StatusUnprocessableEntity,
		wantErr:  "enabled is null\n",
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handlers := map[string]http.Handler{}

			conf := Config{
				Filename: filepath.Join(t.TempDir(), "stats.db"),
				Limit:    time.Hour * 24,
				Enabled:  true,
				UnitID:   func() (id uint32) { return 0 },
				HTTPRegister: func(
					_ string,
					url string,
					handler http.HandlerFunc,
				) {
					handlers[url] = handler
				},
				ConfigModified: func() {},
			}

			s, err := New(conf)
			require.NoError(t, err)

			s.Start()
			testutil.CleanupAndRequireSuccess(t, s.Close)

			buf, err := json.Marshal(tc.body)
			require.NoError(t, err)

			const (
				configGet = "/control/stats/config"
				configPut = "/control/stats/config/update"
			)

			req := httptest.NewRequest(http.MethodPut, configPut, bytes.NewReader(buf))
			rw := httptest.NewRecorder()

			handlers[configPut].ServeHTTP(rw, req)
			require.Equal(t, tc.wantCode, rw.Code)

			if tc.wantCode != http.StatusOK {
				assert.Equal(t, tc.wantCode, rw.Code)
				assert.Equal(t, tc.wantErr, rw.Body.String())

				return
			}

			resp := httptest.NewRequest(http.MethodGet, configGet, nil)
			rw = httptest.NewRecorder()

			handlers[configGet].ServeHTTP(rw, resp)
			require.Equal(t, http.StatusOK, rw.Code)

			ans := getConfigResp{}
			jerr := json.Unmarshal(rw.Body.Bytes(), &ans)
			require.NoError(t, jerr)

			assert.Equal(t, tc.body, ans)
		})
	}
}
