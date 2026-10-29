package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type BlocklistConfig struct {
	Name    string `yaml:"name" json:"name"`
	URL     string `yaml:"url" json:"url"`
	Enabled bool   `yaml:"enabled" json:"enabled"`
}

type Config struct {
	mu sync.RWMutex

	Server struct {
		DNSAddress     string   `yaml:"dns_address" json:"dns_address"`                       // default ":53"
		WebAddress     string   `yaml:"web_address" json:"web_address"`                       // default ":8080"
		BlockingOn     bool     `yaml:"blocking_on" json:"blocking_on"`                       // global toggle
		BlockTTL       uint32   `yaml:"block_ttl" json:"block_ttl"`                           // default 10s
		PauseTimeout   int64    `yaml:"pause_timeout" json:"pause_timeout"`                   // timestamp until when blocking is paused
		BlockMode      string   `yaml:"block_mode" json:"block_mode"`                         // "block_page" ou "null_ip"
		ServerIP       string   `yaml:"server_ip" json:"server_ip"`                           // IP do servidor CoorpDNS na rede local
		PublicURL      string   `yaml:"public_url,omitempty" json:"public_url,omitempty"`     // URL pública opcional (ex: https://dns.empresa.com.br)
		TrustedProxies []string `yaml:"trusted_proxies,omitempty" json:"trusted_proxies"`     // CIDRs/IPs de proxies reversos confiáveis
		SupportEmail   string   `yaml:"support_email" json:"support_email"`                   // E-mail de suporte da TI
		SupportPhone   string   `yaml:"support_phone" json:"support_phone"`                   // WhatsApp ou Telefone de suporte
		OrgName        string   `yaml:"org_name" json:"org_name"`                             // Nome da empresa ou departamento
	} `yaml:"server" json:"server"`

	Upstreams []string `yaml:"upstreams" json:"upstreams"`

	Storage struct {
		DBPath           string `yaml:"db_path" json:"db_path"`
		HistoryDays      int    `yaml:"history_days" json:"history_days"`           // 180 days aggregation
		RawRetentionDays int    `yaml:"raw_retention_days" json:"raw_retention_days"` // 14 days detailed logs
	} `yaml:"storage" json:"storage"`

	Blocklists  []BlocklistConfig `yaml:"blocklists" json:"blocklists"`
	CustomRules []string          `yaml:"custom_rules" json:"custom_rules"`
	AllowRules  []string          `yaml:"allow_rules" json:"allow_rules"`
}

func DefaultConfig() *Config {
	cfg := &Config{}
	cfg.Server.DNSAddress = ":53"
	cfg.Server.WebAddress = ":8080"
	cfg.Server.BlockingOn = true
	cfg.Server.BlockTTL = 10
	cfg.Server.PauseTimeout = 0
	cfg.Server.BlockMode = "block_page"
	cfg.Server.ServerIP = "192.168.1.100"
	cfg.Server.PublicURL = ""
	cfg.Server.TrustedProxies = []string{"127.0.0.1/32", "::1/128"}
	cfg.Server.SupportEmail = "suporte@empresa.com.br"
	cfg.Server.SupportPhone = "5511999998888"
	cfg.Server.OrgName = "Segurança da Informação & TI"

	cfg.Upstreams = []string{
		"https://cloudflare-dns.com/dns-query",
		"https://dns.quad9.net/dns-query",
		"tls://1.1.1.1:853",
		"1.1.1.1:53",
	}

	cfg.Storage.DBPath = "./data/coorpdns.db"
	cfg.Storage.HistoryDays = 180
	cfg.Storage.RawRetentionDays = 14

	cfg.Blocklists = []BlocklistConfig{
		{
			Name:    "OISD Basic (Leve e Seguro)",
			URL:     "https://small.oisd.nl",
			Enabled: true,
		},
		{
			Name:    "StevenBlack Unified Hosts (Adware + Malware)",
			URL:     "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts",
			Enabled: true,
		},
		{
			Name:    "HaGeZi Multi Light",
			URL:     "https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/light.txt",
			Enabled: true,
		},
	}

	cfg.CustomRules = []string{}
	cfg.AllowRules = []string{}

	return cfg
}

func LoadConfig(path string) (*Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		cfg := DefaultConfig()
		if err := cfg.Save(path); err != nil {
			return nil, err
		}
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) Save(path string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func (c *Config) IsBlockingActive() bool {
	active, _ := c.IsBlockingEffective()
	return active
}

// IsBlockingEffective avalia de forma thread-safe se o bloqueio está ativo considerando pausas temporárias.
func (c *Config) IsBlockingEffective() (bool, int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Server.PauseTimeout > 0 {
		now := time.Now().Unix()
		if now < c.Server.PauseTimeout {
			return false, c.Server.PauseTimeout - now
		}
		c.Server.PauseTimeout = 0
	}
	return c.Server.BlockingOn, 0
}

func (c *Config) SetBlocking(active bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Server.BlockingOn = active
	if active {
		c.Server.PauseTimeout = 0
	}
}

func (c *Config) SetBlockingState(enabled bool, pauseDurationSeconds int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if enabled {
		c.Server.BlockingOn = true
		c.Server.PauseTimeout = 0
		return
	}
	if pauseDurationSeconds > 0 {
		c.Server.BlockingOn = true
		c.Server.PauseTimeout = time.Now().Unix() + pauseDurationSeconds
	} else {
		c.Server.BlockingOn = false
		c.Server.PauseTimeout = 0
	}
}

func (c *Config) GetRules() (custom []string, allow []string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	custom = append([]string{}, c.CustomRules...)
	allow = append([]string{}, c.AllowRules...)
	return custom, allow
}

func removeRuleSlice(slice []string, target string) []string {
	out := make([]string, 0, len(slice))
	for _, item := range slice {
		if !strings.EqualFold(strings.TrimSpace(item), target) {
			out = append(out, item)
		}
	}
	return out
}

func appendUniqueRule(slice []string, rule string) []string {
	for _, item := range slice {
		if strings.EqualFold(strings.TrimSpace(item), rule) {
			return slice
		}
	}
	return append(slice, rule)
}

func (c *Config) ModifyRule(action, rule string) (custom []string, allow []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	rule = strings.TrimSpace(rule)
	switch action {
	case "add_block":
		c.CustomRules = appendUniqueRule(c.CustomRules, rule)
	case "remove_block":
		c.CustomRules = removeRuleSlice(c.CustomRules, rule)
	case "add_allow":
		c.AllowRules = appendUniqueRule(c.AllowRules, rule)
	case "remove_allow":
		c.AllowRules = removeRuleSlice(c.AllowRules, rule)
	}

	custom = append([]string{}, c.CustomRules...)
	allow = append([]string{}, c.AllowRules...)
	return custom, allow
}

func (c *Config) GetSupportInfo() (blockMode, serverIP, publicURL, email, phone, org string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Server.BlockMode, c.Server.ServerIP, c.Server.PublicURL, c.Server.SupportEmail, c.Server.SupportPhone, c.Server.OrgName
}

func (c *Config) UpdateSupportInfo(blockMode, serverIP, publicURL, email, phone, org string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if blockMode == "block_page" || blockMode == "null_ip" {
		c.Server.BlockMode = blockMode
	}
	if serverIP != "" {
		c.Server.ServerIP = serverIP
	}
	if publicURL != "" {
		c.Server.PublicURL = publicURL
	}
	if email != "" {
		c.Server.SupportEmail = email
	}
	if phone != "" {
		c.Server.SupportPhone = phone
	}
	if org != "" {
		c.Server.OrgName = org
	}
}

func (c *Config) GetTrustedProxies() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, len(c.Server.TrustedProxies))
	copy(out, c.Server.TrustedProxies)
	return out
}

func (c *Config) GetBlockTTL() uint32 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.Server.BlockTTL == 0 {
		return 10
	}
	return c.Server.BlockTTL
}
