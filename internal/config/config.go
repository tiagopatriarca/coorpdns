package config

import (
	"os"
	"path/filepath"
	"sync"

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
		DNSAddress   string `yaml:"dns_address" json:"dns_address"`     // default ":53"
		WebAddress   string `yaml:"web_address" json:"web_address"`     // default ":8080"
		BlockingOn   bool   `yaml:"blocking_on" json:"blocking_on"`     // global toggle
		BlockTTL     uint32 `yaml:"block_ttl" json:"block_ttl"`         // default 10s
		PauseTimeout int64  `yaml:"pause_timeout" json:"pause_timeout"` // timestamp until when blocking is paused
		BlockMode    string `yaml:"block_mode" json:"block_mode"`       // "block_page" ou "null_ip"
		ServerIP     string `yaml:"server_ip" json:"server_ip"`         // IP do servidor CoorpDNS na rede local
		SupportEmail string `yaml:"support_email" json:"support_email"` // E-mail de suporte da TI
		SupportPhone string `yaml:"support_phone" json:"support_phone"` // WhatsApp ou Telefone de suporte
		OrgName      string `yaml:"org_name" json:"org_name"`           // Nome da empresa ou departamento
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
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (c *Config) IsBlockingActive() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Server.BlockingOn
}

func (c *Config) SetBlocking(active bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Server.BlockingOn = active
}
