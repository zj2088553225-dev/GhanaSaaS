package service

import "testing"

func TestClientIPResolverTrustBoundaryAndNormalization(t *testing.T) {
	resolver, err := NewClientIPResolver([]string{
		"172.16.0.0/12",
		"10.0.0.0/8",
		"2001:db8:1::/48",
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		remote string
		cfIP   string
		realIP string
		xff    []string
		want   string
	}{
		{
			name: "untrusted peer cannot forge headers", remote: "198.51.100.20:443",
			cfIP: "203.0.113.1", realIP: "203.0.113.2", xff: []string{"203.0.113.3"}, want: "198.51.100.20",
		},
		{
			name: "trusted proxy accepts sanitized cloudflare header", remote: "172.18.0.4:8080",
			cfIP: "203.0.113.10", xff: []string{"192.0.2.8"}, want: "203.0.113.10",
		},
		{
			name: "multi layer xff walks right to left", remote: "172.18.0.4:8080",
			xff: []string{"192.0.2.44, 198.51.100.50, 10.0.0.7"}, want: "198.51.100.50",
		},
		{
			name: "x real fallback", remote: "172.18.0.4:8080",
			realIP: "203.0.113.20", want: "203.0.113.20",
		},
		{
			name: "mapped ipv4 is normalized", remote: "[::ffff:203.0.113.21]:8080",
			want: "203.0.113.21",
		},
		{
			name: "ipv6 is preserved", remote: "[2001:db8:2::7]:8080",
			want: "2001:db8:2::7",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := resolver.Resolve(testCase.remote, testCase.cfIP, testCase.realIP, testCase.xff...); got != testCase.want {
				t.Fatalf("Resolve() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestClientIPResolverRejectsUnsafeCIDR(t *testing.T) {
	if _, err := NewClientIPResolver([]string{"0.0.0.0/0"}); err == nil {
		t.Fatal("trust-all IPv4 CIDR was accepted")
	}
	if _, err := NewClientIPResolver([]string{"::/0"}); err == nil {
		t.Fatal("trust-all IPv6 CIDR was accepted")
	}
}
