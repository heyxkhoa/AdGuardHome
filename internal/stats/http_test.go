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

func TestHandleStatsV2(t *testing.T) {
	var (
		halfHour = time.Hour / 2
		hour     = time.Hour
		year     = timeutil.Day * 365
	)

	testCases := []struct {
		name     string
		body     configRespV2
		wantCode int
		wantErr  string
	}{{
		name: "set_ivl_1_hour",
		body: configRespV2{
			Enabled:  aghalg.NBTrue,
			Interval: float64(hour.Milliseconds()),
			Ignored:  nil,
		},
		wantCode: http.StatusOK,
		wantErr:  "",
	}, {
		name: "small_interval",
		body: configRespV2{
			Enabled:  aghalg.NBTrue,
			Interval: float64(halfHour.Milliseconds()),
			Ignored:  []string{},
		},
		wantCode: http.StatusBadRequest,
		wantErr:  "unsupported interval\n",
	}, {
		name: "big_interval",
		body: configRespV2{
			Enabled:  aghalg.NBTrue,
			Interval: float64(year.Milliseconds() + hour.Milliseconds()),
			Ignored:  []string{},
		},
		wantCode: http.StatusBadRequest,
		wantErr:  "unsupported interval\n",
	}, {
		name: "set_ignored_ivl_1_year",
		body: configRespV2{
			Enabled:  aghalg.NBTrue,
			Interval: float64(year.Milliseconds()),
			Ignored: []string{
				"ignor.ed",
				"ignored.to",
			},
		},
		wantCode: http.StatusOK,
		wantErr:  "",
	}, {
		name: "ignored_duplicate",
		body: configRespV2{
			Enabled:  aghalg.NBTrue,
			Interval: float64(hour.Milliseconds()),
			Ignored: []string{
				"ignor.ed",
				"ignor.ed",
			},
		},
		wantCode: http.StatusBadRequest,
		wantErr:  "ignored: duplicate host name \"ignor.ed\"\n",
	}, {
		name: "ignored_empty",
		body: configRespV2{
			Enabled:  aghalg.NBTrue,
			Interval: float64(hour.Milliseconds()),
			Ignored: []string{
				"",
			},
		},
		wantCode: http.StatusBadRequest,
		wantErr:  "ignored: host name is empty\n",
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
				assert.Equal(t, tc.wantErr, rw.Body.String())

				return
			}

			resp := httptest.NewRequest(http.MethodGet, configGet, nil)
			rw = httptest.NewRecorder()

			handlers[configGet].ServeHTTP(rw, resp)
			require.Equal(t, http.StatusOK, rw.Code)

			ans := configRespV2{}
			jerr := json.Unmarshal(rw.Body.Bytes(), &ans)
			require.NoError(t, jerr)

			assert.Equal(t, tc.body, ans)
		})
	}
}
