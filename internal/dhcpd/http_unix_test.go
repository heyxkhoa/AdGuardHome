//go:build darwin || freebsd || linux || openbsd

package dhcpd

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer_handleDHCPStatus(t *testing.T) {
	const (
		staticName  = "static-client"
		anotherName = "another-client"
	)

	staticIP := netip.MustParseAddr("192.168.10.10")
	// anotherIP := DefaultRangeStart
	staticMAC := net.HardwareAddr{0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA}
	// anotherMAC := net.HardwareAddr{0xBB, 0xBB, 0xBB, 0xBB, 0xBB, 0xBB}

	conf4 := defaultV4ServerConf()
	conf4.LeaseDuration = 86400

	serverConf := &ServerConfig{
		Enabled:        true,
		Conf4:          *conf4,
		WorkDir:        t.TempDir(),
		DBFilePath:     dbFilename,
		ConfigModified: func() {},
	}

	wantConf := dhcpStatusResponse{
		IfaceName:    "",
		V4:           *conf4,
		V6:           V6ServerConf{},
		Leases:       []*Lease{},
		StaticLeases: []*Lease{},
		Enabled:      true,
	}

	wantLease := &Lease{
		Expiry:   time.Unix(leaseExpireStatic, 0),
		Hostname: staticName,
		HWAddr:   staticMAC,
		IP:       staticIP,
	}

	s, err := Create(serverConf)
	require.NoError(t, err)

	t.Run("status", func(t *testing.T) {
		w := httptest.NewRecorder()
		var r *http.Request
		r, err = http.NewRequest(http.MethodGet, "", nil)
		require.NoError(t, err)

		wantResp := dhcpStatusResponse{
			IfaceName:    "",
			V4:           *conf4,
			V6:           V6ServerConf{},
			Leases:       []*Lease{},
			StaticLeases: []*Lease{},
			Enabled:      true,
		}

		b := &bytes.Buffer{}
		err = json.NewEncoder(b).Encode(&wantResp)
		require.NoError(t, err)

		s.handleDHCPStatus(w, r)
		assert.Equal(t, http.StatusOK, w.Code)

		assert.JSONEq(t, b.String(), w.Body.String())
	})

	t.Run("add_static_lease", func(t *testing.T) {
		w := httptest.NewRecorder()

		b := &bytes.Buffer{}
		err = json.NewEncoder(b).Encode(&wantLease)
		require.NoError(t, err)

		var r *http.Request
		r, err = http.NewRequest(http.MethodPost, "", b)
		require.NoError(t, err)

		s.handleDHCPAddStaticLease(w, r)
		assert.Equal(t, http.StatusOK, w.Code)

		wantConf := wantConf
		wantConf.StaticLeases = []*Lease{wantLease}

		err = json.NewEncoder(b).Encode(&wantConf)
		require.NoError(t, err)

		s.handleDHCPStatus(w, r)
		assert.Equal(t, http.StatusOK, w.Code)

		assert.JSONEq(t, b.String(), w.Body.String())
	})

	t.Run("remove_static_lease", func(t *testing.T) {
		w := httptest.NewRecorder()

		b := &bytes.Buffer{}
		err = json.NewEncoder(b).Encode(&wantLease)
		require.NoError(t, err)

		var r *http.Request
		r, err = http.NewRequest(http.MethodPost, "", b)
		require.NoError(t, err)

		s.handleDHCPRemoveStaticLease(w, r)
		assert.Equal(t, http.StatusOK, w.Code)

		err = json.NewEncoder(b).Encode(&wantConf)
		require.NoError(t, err)

		s.handleDHCPStatus(w, r)
		assert.Equal(t, http.StatusOK, w.Code)

		assert.JSONEq(t, b.String(), w.Body.String())
	})

	t.Run("set_config", func(t *testing.T) {
		w := httptest.NewRecorder()

		conf := wantConf
		conf.Enabled = false

		b := &bytes.Buffer{}
		err = json.NewEncoder(b).Encode(&conf)
		require.NoError(t, err)

		var r *http.Request
		r, err = http.NewRequest(http.MethodPost, "", b)
		require.NoError(t, err)

		s.handleDHCPSetConfig(w, r)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}
