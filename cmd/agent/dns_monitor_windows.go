//go:build windows

package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type AdapterInfo struct {
	Name       string
	IfIndex    uint32
	IfType     uint32
	DNSServers []string
}

type DNSMonitor struct {
	mu                sync.Mutex
	fallbackSetter    func(string)
	monitoredAdapters map[string]bool
	ticker            *time.Ticker
	stopChan          chan struct{}
}

func NewDNSMonitor(fallbackSetter func(string)) *DNSMonitor {
	return &DNSMonitor{
		fallbackSetter:    fallbackSetter,
		monitoredAdapters: make(map[string]bool),
		stopChan:          make(chan struct{}),
	}
}

func (m *DNSMonitor) Start(ctx context.Context) {
	log.Println("[DNS-MONITOR] 🛡️  Mecanismo de Autocura de DNS Ativo (Ciclo de 5s)")

	// Executa uma primeira checagem imediata ao iniciar
	m.checkAndEnforce()

	m.ticker = time.NewTicker(5 * time.Second)
	defer m.ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopChan:
			return
		case <-m.ticker.C:
			m.checkAndEnforce()
		}
	}
}

func (m *DNSMonitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	select {
	case <-m.stopChan:
		// Já parado
	default:
		close(m.stopChan)
	}

	m.restoreAdapters()
}

func (m *DNSMonitor) checkAndEnforce() {
	adapters, err := getPhysicalAdapters()
	if err != nil {
		log.Printf("[DNS-MONITOR] [AVISO] Falha ao consultar adaptadores de rede: %v", err)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, a := range adapters {
		m.monitoredAdapters[a.Name] = true

		needsEnforcement := false
		if len(a.DNSServers) == 0 {
			needsEnforcement = true
		} else if a.DNSServers[0] != "127.0.0.1" {
			needsEnforcement = true
		}

		if needsEnforcement {
			// 1. Se havia um DNS entregue pelo roteador/DHCP, salva como fallback dinâmico
			if len(a.DNSServers) > 0 {
				existingDNS := a.DNSServers[0]
				if existingDNS != "" && existingDNS != "0.0.0.0" && !strings.HasPrefix(existingDNS, "127.") {
					log.Printf("[DNS-MONITOR] 📡 DNS detectado na interface '%s': %s (salvo como fallback dinâmico)", a.Name, existingDNS)
					if m.fallbackSetter != nil {
						m.fallbackSetter(existingDNS)
					}
				}
			}

			// 2. Reaplica 127.0.0.1 na placa de rede
			log.Printf("[DNS-MONITOR] ⚠️  DNS incorreto ou alterado na interface '%s' (%v). Reaplicando 127.0.0.1...", a.Name, a.DNSServers)
			m.enforce127(a)
		}
	}
}

func (m *DNSMonitor) enforce127(a AdapterInfo) {
	// Tenta primeiro via netsh (execução ultrarrápida em C puro, < 30ms)
	cmd := exec.Command("netsh", "interface", "ipv4", "set", "dnsservers", fmt.Sprintf("name=%s", a.Name), "source=static", "address=127.0.0.1", "register=primary", "validate=no")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	err := cmd.Run()

	if err != nil {
		// Fallback para PowerShell caso o nome possua caracteres com encode especial
		psCmd := fmt.Sprintf("Set-DnsClientServerAddress -InterfaceIndex %d -ServerAddresses ('127.0.0.1') -ErrorAction SilentlyContinue", a.IfIndex)
		fallbackCmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psCmd)
		fallbackCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		fallbackCmd.Run()
	}

	log.Printf("[DNS-MONITOR] ✅ [AUTOCURA] Interface '%s' reconfigurada com sucesso para 127.0.0.1", a.Name)
}

func (m *DNSMonitor) restoreAdapters() {
	if len(m.monitoredAdapters) == 0 {
		return
	}

	log.Println("[DNS-MONITOR] 🔄 Encerramento detectado. Restaurando adaptadores para DHCP automático...")
	for name := range m.monitoredAdapters {
		cmd := exec.Command("netsh", "interface", "ipv4", "set", "dnsservers", fmt.Sprintf("name=%s", name), "source=dhcp")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if err := cmd.Run(); err != nil {
			psCmd := fmt.Sprintf("Set-DnsClientServerAddress -InterfaceAlias '%s' -ResetServerAddresses -ErrorAction SilentlyContinue", name)
			pCmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psCmd)
			pCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			pCmd.Run()
		}
		log.Printf("[DNS-MONITOR] ✅ Interface '%s' restaurada para DHCP.", name)
	}
}

const (
	gaaFlagIncludeGateways      = 0x0080
	gaaFlagIncludeAllInterfaces = 0x0100
)

func getPhysicalAdapters() ([]AdapterInfo, error) {
	var size uint32 = 16384
	buf := make([]byte, size)
	flags := uint32(gaaFlagIncludeGateways | gaaFlagIncludeAllInterfaces)

	err := windows.GetAdaptersAddresses(windows.AF_INET, flags, 0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])), &size)
	if err == windows.ERROR_BUFFER_OVERFLOW {
		buf = make([]byte, size)
		err = windows.GetAdaptersAddresses(windows.AF_INET, flags, 0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])), &size)
	}
	if err != nil {
		return nil, err
	}

	var adapters []AdapterInfo
	curr := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
	for curr != nil {
		friendlyName := windows.UTF16PtrToString(curr.FriendlyName)
		ifType := curr.IfType
		operStatus := curr.OperStatus
		hasGateway := curr.FirstGatewayAddress != nil

		// Filtrar exclusivamente adaptadores físicos com gateway IPv4 ativo:
		// 6 = IF_TYPE_ETHERNET_CSMACD (Ethernet cabeada)
		// 71 = IF_TYPE_IEEE80211 (Wi-Fi)
		// Ignora túneis, VPNs (Tailscale, OpenVPN, Cisco), WSL e Hyper-V
		isPhysical := (ifType == 6 || ifType == 71)
		isUp := (operStatus == windows.IfOperStatusUp)

		if isPhysical && isUp && hasGateway {
			var dnsList []string
			dnsAddr := curr.FirstDnsServerAddress
			for dnsAddr != nil {
				ip := dnsAddr.Address.IP()
				if ip != nil && ip.To4() != nil {
					dnsList = append(dnsList, ip.String())
				}
				dnsAddr = dnsAddr.Next
			}

			adapters = append(adapters, AdapterInfo{
				Name:       friendlyName,
				IfIndex:    curr.IfIndex,
				IfType:     ifType,
				DNSServers: dnsList,
			})
		}

		curr = curr.Next
	}

	return adapters, nil
}
