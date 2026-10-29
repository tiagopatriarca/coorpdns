package web

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const maxRateLimiterKeys = 10000

// LoginRateLimiter implementa proteção contra ataques de força bruta, credential stuffing e enumeração de tokens
type LoginRateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	stopChan chan struct{}
}

func NewLoginRateLimiter() *LoginRateLimiter {
	l := &LoginRateLimiter{
		attempts: make(map[string][]time.Time),
		stopChan: make(chan struct{}),
	}
	go l.cleanupLoop()
	return l
}

func (l *LoginRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			l.cleanup()
		case <-l.stopChan:
			return
		}
	}
}

func (l *LoginRateLimiter) cleanupLocked(now time.Time) {
	cutoff := now.Add(-10 * time.Minute)
	for key, list := range l.attempts {
		var active []time.Time
		for _, t := range list {
			if t.After(cutoff) {
				active = append(active, t)
			}
		}
		if len(active) == 0 {
			delete(l.attempts, key)
		} else {
			l.attempts[key] = active
		}
	}
}

func (l *LoginRateLimiter) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cleanupLocked(time.Now())
}

// Allow verifica se a chave atingiu o limite padrão (máximo 5 tentativas com falha por 5 minutos)
func (l *LoginRateLimiter) Allow(key string) bool {
	return l.AllowWithLimit(key, 5, 5*time.Minute)
}

// AllowWithLimit verifica se a chave atingiu um limite customizado dentro da janela informada
func (l *LoginRateLimiter) AllowWithLimit(key string, maxAttempts int, window time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-window)

	var recent []time.Time
	for _, t := range l.attempts[key] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	if len(recent) == 0 {
		delete(l.attempts, key)
	} else {
		l.attempts[key] = recent
	}

	return len(recent) < maxAttempts
}

// RecordFailedAttempt registra uma falha de autenticação protegendo contra exaustão de memória
func (l *LoginRateLimiter) RecordFailedAttempt(key string) {
	if key == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	if _, exists := l.attempts[key]; !exists && len(l.attempts) >= maxRateLimiterKeys {
		l.cleanupLocked(now)
		if len(l.attempts) >= maxRateLimiterKeys {
			for k := range l.attempts {
				delete(l.attempts, k)
				break
			}
		}
	}
	l.attempts[key] = append(l.attempts[key], now)
}

// Reset limpa o histórico de falhas ao obter sucesso na autenticação
func (l *LoginRateLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}

func isTrustedProxy(remoteHost string, trustedProxies []string) bool {
	ip := net.ParseIP(strings.TrimSpace(remoteHost))
	if ip == nil {
		return false
	}
	if len(trustedProxies) == 0 {
		return ip.IsLoopback()
	}
	for _, entry := range trustedProxies {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			if _, cidr, err := net.ParseCIDR(entry); err == nil && cidr.Contains(ip) {
				return true
			}
		} else if parsed := net.ParseIP(entry); parsed != nil && parsed.Equal(ip) {
			return true
		}
	}
	return false
}

// getRealClientIP extrai o IP real do cliente apenas aceitando X-Forwarded-For / X-Real-IP de proxies reversos confiáveis
func getRealClientIP(r *http.Request, trustedProxies ...[]string) string {
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || remoteHost == "" {
		remoteHost = strings.TrimSpace(r.RemoteAddr)
	}

	var proxies []string
	if len(trustedProxies) > 0 {
		proxies = trustedProxies[0]
	}

	if isTrustedProxy(remoteHost, proxies) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			// Percorre da direita para a esquerda para obter o primeiro IP não-proxy válido (anti-spoofing XFF)
			for i := len(parts) - 1; i >= 0; i-- {
				candidate := strings.TrimSpace(parts[i])
				if parsed := net.ParseIP(candidate); parsed != nil {
					if !isTrustedProxy(candidate, proxies) || i == 0 {
						return parsed.String()
					}
				}
			}
		}
		if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
			if parsed := net.ParseIP(xri); parsed != nil {
				return parsed.String()
			}
		}
	}

	return remoteHost
}

func isSecureRequest(r *http.Request, trustedProxies ...[]string) bool {
	if r.TLS != nil {
		return true
	}
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || remoteHost == "" {
		remoteHost = strings.TrimSpace(r.RemoteAddr)
	}
	var proxies []string
	if len(trustedProxies) > 0 {
		proxies = trustedProxies[0]
	}
	if isTrustedProxy(remoteHost, proxies) {
		return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
	}
	return false
}
