// +build aix darwin dragonfly freebsd linux netbsd openbsd solaris

package dhcpd

import (
	"net"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func notify4(flags uint32) {
}

func TestV4_AddRemove_static(t *testing.T) {
	s, err := v4Create(V4ServerConf{
		Enabled:    true,
		RangeStart: net.IP{192, 168, 10, 100},
		RangeEnd:   net.IP{192, 168, 10, 200},
		GatewayIP:  net.IP{192, 168, 10, 1},
		SubnetMask: net.IP{255, 255, 255, 0},
		notify:     notify4,
	})
	require.Nil(t, err)

	ls := s.GetLeases(LeasesStatic)
	assert.Empty(t, ls)

	// Add static lease.
	l := Lease{
		IP:     net.IP{192, 168, 10, 150},
		HWAddr: net.HardwareAddr{0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA},
	}
	require.Nil(t, s.AddStaticLease(l))
	assert.NotNil(t, s.AddStaticLease(l))

	ls = s.GetLeases(LeasesStatic)
	require.Len(t, ls, 1)
	assert.True(t, l.IP.Equal(ls[0].IP))
	assert.Equal(t, l.HWAddr, ls[0].HWAddr)
	assert.EqualValues(t, leaseExpireStatic, ls[0].Expiry.Unix())

	// Try to remove static lease.
	assert.NotNil(t, s.RemoveStaticLease(Lease{
		IP:     net.IP{192, 168, 10, 110},
		HWAddr: net.HardwareAddr{0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA},
	}))

	// Remove static lease.
	require.Nil(t, s.RemoveStaticLease(l))
	ls = s.GetLeases(LeasesStatic)
	assert.Empty(t, ls)
}

func TestV4_AddReplace(t *testing.T) {
	sIface, err := v4Create(V4ServerConf{
		Enabled:    true,
		RangeStart: net.IP{192, 168, 10, 100},
		RangeEnd:   net.IP{192, 168, 10, 200},
		GatewayIP:  net.IP{192, 168, 10, 1},
		SubnetMask: net.IP{255, 255, 255, 0},
		notify:     notify4,
	})
	require.Nil(t, err)

	s, ok := sIface.(*v4Server)
	require.True(t, ok)

	dynLeases := []Lease{{
		IP:     net.IP{192, 168, 10, 150},
		HWAddr: net.HardwareAddr{0x11, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA},
	}, {
		IP:     net.IP{192, 168, 10, 151},
		HWAddr: net.HardwareAddr{0x22, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA},
	}}

	for i := range dynLeases {
		s.addLease(&dynLeases[i])
	}

	stLeases := []Lease{{
		IP:     net.IP{192, 168, 10, 150},
		HWAddr: net.HardwareAddr{0x33, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA},
	}, {
		IP:     net.IP{192, 168, 10, 152},
		HWAddr: net.HardwareAddr{0x22, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA},
	}}

	for _, l := range stLeases {
		require.Nil(t, s.AddStaticLease(l))
	}

	ls := s.GetLeases(LeasesStatic)
	require.Len(t, ls, 2)

	for i, l := range ls {
		assert.True(t, stLeases[i].IP.Equal(l.IP))
		assert.Equal(t, stLeases[i].HWAddr, l.HWAddr)
		assert.EqualValues(t, leaseExpireStatic, l.Expiry.Unix())
	}
}

func TestV4StaticLease_Get(t *testing.T) {
	var err error
	sIface, err := v4Create(V4ServerConf{
		Enabled:    true,
		RangeStart: net.IP{192, 168, 10, 100},
		RangeEnd:   net.IP{192, 168, 10, 200},
		GatewayIP:  net.IP{192, 168, 10, 1},
		SubnetMask: net.IP{255, 255, 255, 0},
		notify:     notify4,
	})
	require.Nil(t, err)

	s, ok := sIface.(*v4Server)
	require.True(t, ok)
	s.conf.dnsIPAddrs = []net.IP{{192, 168, 10, 1}}

	l := Lease{
		IP:     net.IP{192, 168, 10, 150},
		HWAddr: net.HardwareAddr{0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA},
	}
	require.Nil(t, s.AddStaticLease(l))

	var req, resp *dhcpv4.DHCPv4
	mac := net.HardwareAddr{0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA}

	t.Run("discover", func(t *testing.T) {
		var err error

		req, err = dhcpv4.NewDiscovery(mac)
		require.Nil(t, err)

		resp, err = dhcpv4.NewReplyFromRequest(req)
		require.Nil(t, err)
		assert.Equal(t, 1, s.process(req, resp))
	})
	require.Nil(t, err)

	t.Run("offer", func(t *testing.T) {
		assert.Equal(t, dhcpv4.MessageTypeOffer, resp.MessageType())
		assert.Equal(t, mac, resp.ClientHWAddr)
		assert.True(t, l.IP.Equal(resp.YourIPAddr))
		assert.True(t, s.conf.GatewayIP.Equal(resp.Router()[0]))
		assert.True(t, s.conf.GatewayIP.Equal(resp.ServerIdentifier()))
		assert.Equal(t, s.conf.subnetMask, resp.SubnetMask())
		assert.Equal(t, s.conf.leaseTime.Seconds(), resp.IPAddressLeaseTime(-1).Seconds())
	})

	t.Run("request", func(t *testing.T) {
		req, err = dhcpv4.NewRequestFromOffer(resp)
		require.Nil(t, err)

		resp, err = dhcpv4.NewReplyFromRequest(req)
		require.Nil(t, err)
		assert.Equal(t, 1, s.process(req, resp))
	})
	require.Nil(t, err)

	t.Run("ack", func(t *testing.T) {
		assert.Equal(t, dhcpv4.MessageTypeAck, resp.MessageType())
		assert.Equal(t, mac, resp.ClientHWAddr)
		assert.True(t, l.IP.Equal(resp.YourIPAddr))
		assert.True(t, s.conf.GatewayIP.Equal(resp.Router()[0]))
		assert.True(t, s.conf.GatewayIP.Equal(resp.ServerIdentifier()))
		assert.Equal(t, s.conf.subnetMask, resp.SubnetMask())
		assert.Equal(t, s.conf.leaseTime.Seconds(), resp.IPAddressLeaseTime(-1).Seconds())
	})

	dnsAddrs := resp.DNS()
	require.Len(t, dnsAddrs, 1)
	assert.True(t, s.conf.GatewayIP.Equal(dnsAddrs[0]))

	t.Run("check_lease", func(t *testing.T) {
		ls := s.GetLeases(LeasesStatic)
		require.Len(t, ls, 1)
		assert.True(t, l.IP.Equal(ls[0].IP))
		assert.Equal(t, mac, ls[0].HWAddr)
	})
}

func TestV4DynamicLease_Get(t *testing.T) {
	var err error
	sIface, err := v4Create(V4ServerConf{
		Enabled:    true,
		RangeStart: net.IP{192, 168, 10, 100},
		RangeEnd:   net.IP{192, 168, 10, 200},
		GatewayIP:  net.IP{192, 168, 10, 1},
		SubnetMask: net.IP{255, 255, 255, 0},
		notify:     notify4,
		Options: []string{
			"81 hex 303132",
			"82 ip 1.2.3.4",
		},
	})
	require.Nil(t, err)

	s, ok := sIface.(*v4Server)
	require.True(t, ok)
	s.conf.dnsIPAddrs = []net.IP{{192, 168, 10, 1}}

	var req, resp *dhcpv4.DHCPv4
	mac := net.HardwareAddr{0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA}

	t.Run("discover", func(t *testing.T) {
		req, err = dhcpv4.NewDiscovery(mac)
		require.Nil(t, err)

		resp, err = dhcpv4.NewReplyFromRequest(req)
		require.Nil(t, err)
		assert.Equal(t, 1, s.process(req, resp))
	})
	require.Nil(t, err)

	t.Run("offer", func(t *testing.T) {
		assert.Equal(t, dhcpv4.MessageTypeOffer, resp.MessageType())
		assert.Equal(t, mac, resp.ClientHWAddr)
		assert.True(t, s.conf.RangeStart.Equal(resp.YourIPAddr))
		assert.True(t, s.conf.GatewayIP.Equal(resp.Router()[0]))
		assert.True(t, s.conf.GatewayIP.Equal(resp.ServerIdentifier()))
		assert.Equal(t, s.conf.subnetMask, resp.SubnetMask())
		assert.Equal(t, s.conf.leaseTime.Seconds(), resp.IPAddressLeaseTime(-1).Seconds())
		assert.Equal(t, []byte("012"), resp.Options[uint8(dhcpv4.OptionFQDN)])
		assert.True(t, net.IP{1, 2, 3, 4}.Equal(net.IP(resp.Options[uint8(dhcpv4.OptionRelayAgentInformation)])))
	})

	t.Run("request", func(t *testing.T) {
		var err error

		req, err = dhcpv4.NewRequestFromOffer(resp)
		require.Nil(t, err)

		resp, err = dhcpv4.NewReplyFromRequest(req)
		require.Nil(t, err)
		assert.Equal(t, 1, s.process(req, resp))
	})
	require.Nil(t, err)

	t.Run("ack", func(t *testing.T) {
		assert.Equal(t, dhcpv4.MessageTypeAck, resp.MessageType())
		assert.Equal(t, mac, resp.ClientHWAddr)
		assert.True(t, s.conf.RangeStart.Equal(resp.YourIPAddr))
		assert.True(t, s.conf.GatewayIP.Equal(resp.Router()[0]))
		assert.True(t, s.conf.GatewayIP.Equal(resp.ServerIdentifier()))
		assert.Equal(t, s.conf.subnetMask, resp.SubnetMask())
		assert.Equal(t, s.conf.leaseTime.Seconds(), resp.IPAddressLeaseTime(-1).Seconds())
	})

	dnsAddrs := resp.DNS()
	require.Len(t, dnsAddrs, 1)
	assert.True(t, net.IP{192, 168, 10, 1}.Equal(dnsAddrs[0]))

	// check lease
	t.Run("check_lease", func(t *testing.T) {
		ls := s.GetLeases(LeasesDynamic)
		assert.Len(t, ls, 1)
		assert.True(t, net.IP{192, 168, 10, 100}.Equal(ls[0].IP))
		assert.Equal(t, mac, ls[0].HWAddr)
	})
}

func TestIP4InRange(t *testing.T) {
	start := net.IP{192, 168, 10, 100}
	stop := net.IP{192, 168, 10, 200}

	testCases := []struct {
		ip   net.IP
		want bool
	}{{
		ip:   net.IP{192, 168, 10, 99},
		want: false,
	}, {
		ip:   net.IP{192, 168, 11, 100},
		want: false,
	}, {
		ip:   net.IP{192, 168, 11, 201},
		want: false,
	}, {
		ip:   start,
		want: true,
	}}

	for _, tc := range testCases {
		t.Run(tc.ip.String(), func(t *testing.T) {
			assert.Equal(t, tc.want, ip4InRange(start, stop, tc.ip))
		})
	}
}
