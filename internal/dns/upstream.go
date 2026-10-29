package dns

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
)

type UpstreamResolver struct {
	upstreams             []string
	counter               uint32
	httpClient            *http.Client
	tlsClient             *dns.Client
	udpClient             *dns.Client
	mu                    sync.RWMutex
	conditionalForwarders map[string]string // domínio -> ip:porta (ex: "empresa.local" -> "192.168.1.10:53")
}

func NewUpstreamResolver(upstreams []string) *UpstreamResolver {
	if len(upstreams) == 0 {
		upstreams = []string{"https://cloudflare-dns.com/dns-query", "1.1.1.1:53"}
	}

	return &UpstreamResolver{
		upstreams:             upstreams,
		conditionalForwarders: make(map[string]string),
		httpClient: &http.Client{
			Timeout: 4 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		tlsClient: &dns.Client{
			Net:     "tcp-tls",
			Timeout: 4 * time.Second,
			TLSConfig: &tls.Config{
				MinVersion:         tls.VersionTLS12,
				InsecureSkipVerify: false,
			},
		},
		udpClient: &dns.Client{
			Net:     "udp",
			Timeout: 3 * time.Second,
		},
	}
}

// Configura encaminhadores condicionais (ex: Active Directory Domain Controllers)
func (u *UpstreamResolver) SetConditionalForwarders(forwarders map[string]string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.conditionalForwarders = make(map[string]string)
	for d, target := range forwarders {
		d = strings.ToLower(strings.TrimSpace(d))
		d = strings.TrimSuffix(d, ".")
		target = strings.TrimSpace(target)
		if !strings.Contains(target, ":") {
			target += ":53"
		}
		u.conditionalForwarders[d] = target
	}
}

func (u *UpstreamResolver) Resolve(req *dns.Msg) (*dns.Msg, string, error) {
	// 1. Checagem de Encaminhamento Condicional (Active Directory / Domínio Interno)
	if len(req.Question) > 0 {
		qname := strings.ToLower(req.Question[0].Name)
		qname = strings.TrimSuffix(qname, ".")

		u.mu.RLock()
		for domain, target := range u.conditionalForwarders {
			if qname == domain || strings.HasSuffix(qname, "."+domain) {
				u.mu.RUnlock()
				resp, err := u.queryTarget(req, target)
				return resp, "ad-dc:" + target, err
			}
		}
		u.mu.RUnlock()
	}

	// 2. Round-robin entre upstreams públicos configurados
	idx := atomic.AddUint32(&u.counter, 1) % uint32(len(u.upstreams))
	target := u.upstreams[idx]

	resp, err := u.queryTarget(req, target)
	if err == nil && resp != nil {
		return resp, target, nil
	}

	// Se falhar, tenta o próximo imediatamente como fallback
	fallbackIdx := (idx + 1) % uint32(len(u.upstreams))
	fallbackTarget := u.upstreams[fallbackIdx]
	resp, err = u.queryTarget(req, fallbackTarget)
	return resp, fallbackTarget, err
}

func (u *UpstreamResolver) queryTarget(req *dns.Msg, target string) (*dns.Msg, error) {
	// 1. Caso DNS-over-HTTPS (DoH - RFC 8484)
	if strings.HasPrefix(target, "https://") {
		return u.queryDoH(req, target)
	}

	// 2. Caso DNS-over-TLS (DoT)
	if strings.HasPrefix(target, "tls://") {
		hostPort := strings.TrimPrefix(target, "tls://")
		if !strings.Contains(hostPort, ":") {
			hostPort += ":853"
		}
		host, _, splitErr := net.SplitHostPort(hostPort)
		if splitErr != nil {
			host = hostPort
		}
		perTargetTLS := &dns.Client{
			Net:     "tcp-tls",
			Timeout: 4 * time.Second,
			TLSConfig: &tls.Config{
				MinVersion:         tls.VersionTLS12,
				InsecureSkipVerify: false,
				ServerName:         host,
			},
		}
		resp, _, err := perTargetTLS.Exchange(req, hostPort)
		return resp, err
	}

	// 3. Caso DNS tradicional (UDP/TCP)
	hostPort := target
	if !strings.Contains(hostPort, ":") {
		hostPort += ":53"
	}

	resp, _, err := u.udpClient.Exchange(req, hostPort)
	if err != nil && resp != nil && resp.Truncated {
		// Se foi truncado via UDP, tenta TCP
		tcpClient := &dns.Client{Net: "tcp", Timeout: 4 * time.Second}
		resp, _, err = tcpClient.Exchange(req, hostPort)
	}
	return resp, err
}

func (u *UpstreamResolver) queryDoH(req *dns.Msg, url string) (*dns.Msg, error) {
	packed, err := req.Pack()
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(packed))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/dns-message")
	httpReq.Header.Set("Accept", "application/dns-message")

	resp, err := u.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DoH HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 65536))
	if err != nil {
		return nil, err
	}

	dnsResp := new(dns.Msg)
	if err := dnsResp.Unpack(body); err != nil {
		return nil, err
	}
	return dnsResp, nil
}
