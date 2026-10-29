package filter

import (
	"net"
	"strings"
	"testing"
)

func TestIsRestrictedIP(t *testing.T) {
	restricted := []string{
		"127.0.0.1",
		"127.0.1.1",
		"::1",
		"0.0.0.0",
		"10.0.0.1",
		"172.16.5.4",
		"192.168.1.100",
		"169.254.169.254", // Cloud Metadata (AWS/GCP/Azure)
		"100.64.0.10",     // CGNAT RFC 6598
		"fe80::1",         // IPv6 Link-Local
		"fc00::1",         // IPv6 Unique Local
	}

	for _, ipStr := range restricted {
		ip := net.ParseIP(ipStr)
		if !isRestrictedIP(ip) {
			t.Errorf("esperava que o IP %s fosse classificado como restrito (anti-SSRF)", ipStr)
		}
	}

	public := []string{
		"1.1.1.1",
		"8.8.8.8",
		"9.9.9.9",
		"2606:4700:4700::1111",
	}

	for _, ipStr := range public {
		ip := net.ParseIP(ipStr)
		if isRestrictedIP(ip) {
			t.Errorf("esperava que o IP público %s fosse permitido", ipStr)
		}
	}
}

func TestFetchListBlocksSSRF(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr string
	}{
		{
			name:    "bloqueia esquema file",
			url:     "file:///etc/passwd",
			wantErr: "esquema não permitido",
		},
		{
			name:    "bloqueia loopback 127.0.0.1",
			url:     "http://127.0.0.1:8080/api/v1/status",
			wantErr: "bloqueio anti-SSRF",
		},
		{
			name:    "bloqueia cloud metadata 169.254.169.254",
			url:     "http://169.254.169.254/latest/meta-data/",
			wantErr: "bloqueio anti-SSRF",
		},
		{
			name:    "bloqueia rede privada RFC1918",
			url:     "http://192.168.1.1/admin",
			wantErr: "bloqueio anti-SSRF",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := FetchList(tc.url)
			if err == nil {
				t.Fatalf("FetchList(%q) deveria falhar, mas retornou nil", tc.url)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("FetchList(%q) erro = %v; esperava conter %q", tc.url, err, tc.wantErr)
			}
		})
	}
}
