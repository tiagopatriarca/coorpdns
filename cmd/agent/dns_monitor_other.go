//go:build !windows

package main

import (
	"context"
	"log"
)

type DNSMonitor struct{}

func NewDNSMonitor(fallbackSetter func(string)) *DNSMonitor {
	return &DNSMonitor{}
}

func (m *DNSMonitor) Start(ctx context.Context) {
	log.Println("[DNS-MONITOR] Autocura de DNS ativa apenas em ambiente Windows.")
}

func (m *DNSMonitor) Stop() {
}
