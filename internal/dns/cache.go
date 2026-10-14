package dns

import (
	"fmt"
	"sync"
	"time"

	"github.com/miekg/dns"
)

type cacheItem struct {
	msg       *dns.Msg
	expiresAt time.Time
	origTTL   uint32
}

type Cache struct {
	mu    sync.RWMutex
	items map[string]*cacheItem
}

func NewCache() *Cache {
	c := &Cache{
		items: make(map[string]*cacheItem),
	}
	go c.cleanupLoop()
	return c
}

func key(name string, qtype uint16) string {
	return fmt.Sprintf("%s:%d", name, qtype)
}

func (c *Cache) Get(name string, qtype uint16) *dns.Msg {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, ok := c.items[key(name, qtype)]
	if !ok {
		return nil
	}

	remaining := time.Until(item.expiresAt)
	if remaining <= 0 {
		return nil
	}

	// Clona mensagem e ajusta TTL
	resp := item.msg.Copy()
	remSeconds := uint32(remaining.Seconds())
	for _, rr := range resp.Answer {
		rr.Header().Ttl = remSeconds
	}
	return resp
}

func (c *Cache) Set(name string, qtype uint16, msg *dns.Msg) {
	if msg == nil || len(msg.Answer) == 0 {
		return
	}

	// Encontra menor TTL na resposta
	minTTL := uint32(3600)
	for _, rr := range msg.Answer {
		if rr.Header().Ttl < minTTL {
			minTTL = rr.Header().Ttl
		}
	}
	if minTTL < 5 {
		minTTL = 5 // Mínimo de 5s para evitar flood
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Evita estouro de memória limitando a 50.000 entradas
	if len(c.items) > 50000 {
		// Limpa metade das chaves
		for k := range c.items {
			delete(c.items, k)
			if len(c.items) < 25000 {
				break
			}
		}
	}

	c.items[key(name, qtype)] = &cacheItem{
		msg:       msg.Copy(),
		expiresAt: time.Now().Add(time.Duration(minTTL) * time.Second),
		origTTL:   minTTL,
	}
}

func (c *Cache) cleanupLoop() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		c.mu.Lock()
		for k, item := range c.items {
			if now.After(item.expiresAt) {
				delete(c.items, k)
			}
		}
		c.mu.Unlock()
	}
}
