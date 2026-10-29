package main

import (
	"embed"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coorpdns/coorpdns/internal/config"
	"github.com/coorpdns/coorpdns/internal/dns"
	"github.com/coorpdns/coorpdns/internal/filter"
	"github.com/coorpdns/coorpdns/internal/storage"
	"github.com/coorpdns/coorpdns/internal/web"
)

//go:embed all:static
var staticFS embed.FS

func main() {
	configPath := flag.String("config", "./config/config.yaml", "Caminho do arquivo de configuração YAML")
	flag.Parse()

	log.Println("==================================================")
	log.Println("    CoorpDNS - Servidor DNS Corporativo & Auditoria")
	log.Println("==================================================")

	// 1. Carrega configuração
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Erro ao carregar configuração: %v", err)
	}
	log.Printf("[CONFIG] Configurações carregadas de %s", *configPath)

	// 2. Inicializa Armazenamento SQLite
	store, err := storage.NewDB(cfg.Storage.DBPath, cfg.Storage.HistoryDays, cfg.Storage.RawRetentionDays)
	if err != nil {
		log.Fatalf("Erro ao inicializar banco de dados: %v", err)
	}
	defer store.Close()
	log.Printf("[STORAGE] Banco SQLite inicializado em %s (Retenção: %d dias)", cfg.Storage.DBPath, cfg.Storage.HistoryDays)

	// 3. Inicializa Motor de Filtragem
	filterEngine := filter.NewEngine()
	filterEngine.LoadRules(nil, cfg.CustomRules, cfg.AllowRules)

	// Dispara carregamento das listas em background
	go func() {
		log.Println("[FILTER] Atualizando listas de bloqueio iniciais...")
		var allDomains []string
		for _, bl := range cfg.Blocklists {
			if !bl.Enabled {
				continue
			}
			log.Printf("[FILTER] Baixando lista: %s (%s)", bl.Name, bl.URL)
			domains, err := filter.FetchList(bl.URL)
			if err != nil {
				log.Printf("[FILTER] Aviso: Falha ao baixar lista %s: %v", bl.Name, err)
				continue
			}
			allDomains = append(allDomains, domains...)
			log.Printf("[FILTER] Lista %s carregada com %d regras.", bl.Name, len(domains))
		}
		filterEngine.LoadRules(allDomains, cfg.CustomRules, cfg.AllowRules)
		rulesCount, _ := filterEngine.Stats()
		log.Printf("[FILTER] Motor ativo com %d regras únicas.", rulesCount)
	}()

	// 4. Inicia Servidor DNS
	dnsServer := dns.NewServer(cfg, filterEngine, store)
	if err := dnsServer.Start(); err != nil {
		log.Fatalf("Erro ao iniciar servidor DNS: %v", err)
	}
	log.Printf("[DNS] Servidor escutando em UDP/TCP %s", cfg.Server.DNSAddress)

	// 5. Inicia Servidor Web e API REST
	webServer := web.NewServer(cfg, filterEngine, store, *configPath, staticFS)
	webServer.SetOnForwardersChange(dnsServer.ReloadConditionalForwarders)
	httpServer := &http.Server{
		Addr:              cfg.Server.WebAddress,
		Handler:           webServer.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("[WEB] Painel administrativo acessível em http://localhost%s", cfg.Server.WebAddress)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[WEB] Servidor HTTP finalizado: %v", err)
		}
	}()

	// 6. Graceful Shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("\n[SISTEMA] Encerrando serviços com segurança...")
	dnsServer.Stop()
	httpServer.Close()
	store.Close()
	log.Println("[SISTEMA] CoorpDNS finalizado com sucesso.")
}
