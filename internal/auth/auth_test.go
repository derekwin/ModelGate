package auth

import (
	"testing"

	"modelgate/internal/models"
)

func TestCheckIPAllowedSupportsExactIPCIDRAndWildcard(t *testing.T) {
	tests := []struct {
		name       string
		allowedIPs string
		ip         string
		want       bool
	}{
		{
			name:       "empty allow list",
			allowedIPs: "",
			ip:         "10.0.0.1",
			want:       true,
		},
		{
			name:       "exact ip match",
			allowedIPs: "10.0.0.1,192.168.1.10",
			ip:         "192.168.1.10",
			want:       true,
		},
		{
			name:       "cidr match",
			allowedIPs: "10.0.0.0/24",
			ip:         "10.0.0.42",
			want:       true,
		},
		{
			name:       "wildcard",
			allowedIPs: "*",
			ip:         "203.0.113.9",
			want:       true,
		},
		{
			name:       "reject non matching ip",
			allowedIPs: "10.0.1.0/24",
			ip:         "10.0.2.10",
			want:       false,
		},
		{
			name:       "ignore invalid entry",
			allowedIPs: "not-an-ip,192.168.10.0/24",
			ip:         "192.168.10.9",
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiKey := &models.APIKey{AllowedIPs: tt.allowedIPs}
			got := CheckIPAllowed(apiKey, tt.ip)
			if got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestSplitIPsTrimsWhitespace(t *testing.T) {
	got := splitIPs(" 10.0.0.1 , 192.168.1.0/24 ,* ")
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(got))
	}
	if got[0] != "10.0.0.1" || got[1] != "192.168.1.0/24" || got[2] != "*" {
		t.Fatalf("unexpected split result: %#v", got)
	}
}
