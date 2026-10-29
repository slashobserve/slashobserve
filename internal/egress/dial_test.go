package egress

import (
	"errors"
	"net/netip"
	"testing"
)

func TestBlockedAddresses(t *testing.T) {
	cases := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1", true},
		{"::1", true},
		{"::ffff:127.0.0.1", true},
		{"::ffff:10.1.2.3", true},
		{"10.0.0.1", true},
		{"172.16.5.4", true},
		{"192.168.1.1", true},
		{"fc00::1", true},
		{"169.254.169.254", true},
		{"fe80::1", true},
		{"224.0.0.1", true},
		{"ff02::1", true},
		{"ff01::1", true},
		{"0.0.0.0", true},
		{"::", true},
		{"0.1.2.3", true},
		{"100.64.0.1", true},
		{"192.0.0.8", true},
		{"192.0.2.10", true},
		{"198.18.0.1", true},
		{"198.51.100.7", true},
		{"203.0.113.9", true},
		{"240.0.0.1", true},
		{"255.255.255.255", true},
		{"64:ff9b::a00:1", true},
		{"64:ff9b:1::1", true},
		{"2001:db8::1", true},
		{"2002::1", true},
		{"fec0::1", true},
		{"93.184.216.34", false},
		{"2606:2800:220:1::1", false},
		{"1.1.1.1", false},
		{"::ffff:93.184.216.34", false},
	}
	for _, c := range cases {
		if got := blocked(netip.MustParseAddr(c.addr)); got != c.want {
			t.Errorf("blocked(%s) = %v, want %v", c.addr, got, c.want)
		}
	}
}

func TestRefuseBlockedChecksTheDialedAddress(t *testing.T) {
	cases := []struct {
		address string
		wantErr error
	}{
		{"127.0.0.1:443", ErrBlockedAddress},
		{"[::1]:443", ErrBlockedAddress},
		{"[::ffff:169.254.169.254]:443", ErrBlockedAddress},
		{"not-an-address", ErrBlockedAddress},
		{"93.184.216.34:443", nil},
		{"[2606:2800:220:1::1]:443", nil},
	}
	for _, c := range cases {
		if err := refuseBlocked("tcp", c.address, nil); !errors.Is(err, c.wantErr) {
			t.Errorf("refuseBlocked(%q) = %v, want %v", c.address, err, c.wantErr)
		}
	}
}
