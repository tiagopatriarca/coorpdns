package web

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// LoginRateLimiter implementa proteção contra ataques de força bruta e credential stuffing
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

func (l *LoginRateLimiter) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-10 * time.Minute)

	for ip, list := range l.attempts {
		var active []time.Time
		for _, t := range list {
			if t.After(cutoff) {
				active = append(active, t)
			}
		}
		if len(active) == 0 {
			delete(l.attempts, ip)
		} else {
			l.attempts[ip] = active
		}
	}
}

// Allow verifica se o IP atingiu o limite de tentativas (máximo 5 tentativas com falha por 5 minutos)
func (l *LoginRateLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-5 * time.Minute)

	var recent []time.Time
	for _, t := range l.attempts[ip] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	l.attempts[ip] = recent

	// Bloqueia se houver 5 ou mais tentativas nos últimos 5 minutos
	return len(recent) < 5
}

// RecordFailedAttempt registra uma falha de autenticação para o IP
func (l *LoginRateLimiter) RecordFailedAttempt(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.attempts[ip] = append(l.attempts[ip], time.Now())
}

// Reset limpa o histórico de falhas ao obter sucesso na autenticação
func (l *LoginRateLimiter) Reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, ip)
}

// getRealClientIP extrai o IP real do cliente considerando headers de proxy reverso
func getRealClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		ip := strings.TrimSpace(parts[0])
		if ip != "" {
			return ip
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		ip := strings.TrimSpace(xri)
		if ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}
