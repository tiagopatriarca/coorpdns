//go:build linux

package main

import (
	"bufio"
	"context"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type DNSMonitor struct {
	mu             sync.Mutex
	fallbackSetter func(string)
	stopChan       chan struct{}
	lastIface      string
	resolvBackup   string
}

func NewDNSMonitor(fallbackSetter func(string)) *DNSMonitor {
	return &DNSMonitor{
		fallbackSetter: fallbackSetter,
		stopChan:       make(chan struct{}),
	}
}

func (m *DNSMonitor) Start(ctx context.Context) {
	log.Println("[DNS-MONITOR] 🛡️  Mecanismo de Autocura de DNS Linux Ativo (Ciclo de 5s)")

	m.checkAndEnforce()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopChan:
			return
		case <-ticker.C:
			m.checkAndEnforce()
		}
	}
}

func (m *DNSMonitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	select {
	case <-m.stopChan:
	default:
		close(m.stopChan)
	}

	m.restore()
}

func (m *DNSMonitor) checkAndEnforce() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. Detectar interface ativa com rota padrão
	iface := getActiveLinuxInterface()
	if iface == "" {
		return
	}
	m.lastIface = iface

	// 2. Se resolvectl estiver disponível (Ubuntu/Debian/Fedora modernos com systemd-resolved)
	if hasCommand("resolvectl") {
		out, err := exec.Command("resolvectl", "dns", iface).Output()
		if err == nil {
			strOut := string(out)
			if !strings.Contains(strOut, "127.0.0.1") {
				// Captura DNS anterior para fallback
				for _, part := range strings.Fields(strOut) {
					cleanIP := strings.TrimSpace(part)
					if cleanIP != "" && cleanIP != "127.0.0.1" && !strings.HasPrefix(cleanIP, "127.") && !strings.Contains(cleanIP, ":") && strings.Count(cleanIP, ".") == 3 {
						if m.fallbackSetter != nil {
							m.fallbackSetter(cleanIP)
							log.Printf("[DNS-MONITOR] 📡 DNS detectado no DHCP da interface '%s': %s (salvo como fallback)", iface, cleanIP)
						}
						break
					}
				}

				log.Printf("[DNS-MONITOR] ⚠️  DNS incorreto na interface '%s'. Reaplicando 127.0.0.1 via resolvectl...", iface)
				exec.Command("resolvectl", "dns", iface, "127.0.0.1").Run()
				exec.Command("resolvectl", "domain", iface, "~.").Run()
				log.Printf("[DNS-MONITOR] ✅ [AUTOCURA] Interface '%s' reconfigurada para 127.0.0.1", iface)
			}
		}
		return
	}

	// 3. Fallback para /etc/resolv.conf direto
	m.enforceResolvConf()
}

func (m *DNSMonitor) enforceResolvConf() {
	content, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return
	}
	strContent := string(content)
	if !strings.Contains(strContent, "nameserver 127.0.0.1") {
		// Captura nameserver original para fallback
		scanner := bufio.NewScanner(strings.NewReader(strContent))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "nameserver ") {
				parts := strings.Fields(line)
				if len(parts) >= 2 && parts[1] != "127.0.0.1" && !strings.HasPrefix(parts[1], "127.") {
					if m.fallbackSetter != nil {
						m.fallbackSetter(parts[1])
						log.Printf("[DNS-MONITOR] 📡 DNS detectado em /etc/resolv.conf: %s (salvo como fallback)", parts[1])
					}
					break
				}
			}
		}

		if m.resolvBackup == "" {
			m.resolvBackup = strContent
		}

		newContent := "nameserver 127.0.0.1\n" + strContent
		os.WriteFile("/etc/resolv.conf", []byte(newContent), 0644)
		log.Println("[DNS-MONITOR] ✅ [AUTOCURA] /etc/resolv.conf atualizado com nameserver 127.0.0.1")
	}
}

func (m *DNSMonitor) restore() {
	log.Println("[DNS-MONITOR] 🔄 Encerramento detectado. Restaurando DNS...")
	if m.lastIface != "" && hasCommand("resolvectl") {
		exec.Command("resolvectl", "revert", m.lastIface).Run()
		log.Printf("[DNS-MONITOR] ✅ resolvectl revert executado na interface '%s'.", m.lastIface)
	} else if m.resolvBackup != "" {
		os.WriteFile("/etc/resolv.conf", []byte(m.resolvBackup), 0644)
		log.Println("[DNS-MONITOR] ✅ /etc/resolv.conf restaurado para estado original.")
	}
}

func getActiveLinuxInterface() string {
	// Executa "ip -o route get 8.8.8.8" para encontrar a interface padrão
	out, err := exec.Command("ip", "-o", "route", "get", "8.8.8.8").Output()
	if err == nil {
		fields := strings.Fields(string(out))
		for i, f := range fields {
			if f == "dev" && i+1 < len(fields) {
				return fields[i+1]
			}
		}
	}
	return ""
}

func hasCommand(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}
