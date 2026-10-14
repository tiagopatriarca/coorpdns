package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/miekg/dns"
)

type Agent struct {
	serverURL  string
	token      string
	listenAddr string
	fallback   string
	httpClient *http.Client
	udpClient  *dns.Client
}

func main() {
	serverURL := flag.String("server", "", "URL do servidor CoorpDNS na nuvem (ex: https://dns.suaempresa.com)")
	token := flag.String("token", "", "Token de autenticação do endpoint corporativo cadastrado no painel")
	listenAddr := flag.String("listen", "127.0.0.1:53", "Endereço local para escutar consultas DNS")
	fallback := flag.String("fallback", "1.1.1.1:53", "Servidor DNS de contingência caso a nuvem esteja inatingível")
	flag.Parse()

	if *serverURL == "" || *token == "" {
		fmt.Println("======================================================================")
		fmt.Println("  🛡️  CoorpDNS Client Agent - Proteção e Auditoria Corporativa Remota")
		fmt.Println("======================================================================")
		fmt.Println("Uso obrigatório:")
		fmt.Println("  coorpdns-agent -server https://dns.suaempresa.com -token tok_SEUTOKEN")
		fmt.Println("\nOpções adicionais:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	trimmedURL := strings.TrimRight(*serverURL, "/")

	agent := &Agent{
		serverURL:  trimmedURL,
		token:      *token,
		listenAddr: *listenAddr,
		fallback:   *fallback,
		httpClient: &http.Client{
			Timeout: 4 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        50,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     60 * time.Second,
			},
		},
		udpClient: &dns.Client{
			Net:     "udp",
			Timeout: 2 * time.Second,
		},
	}

	log.Println("======================================================================")
	log.Printf("  🛡️  CoorpDNS Endpoint Agent Iniciado")
	log.Printf("  Servidor Nuvem : %s", agent.serverURL)
	log.Printf("  Token Dispositivo: %s...%s", agent.token[:6], agent.token[len(agent.token)-4:])
	log.Printf("  Escuta Local   : %s (UDP/TCP)", agent.listenAddr)
	log.Println("======================================================================")

	dns.HandleFunc(".", agent.handleDNS)

	udpServer := &dns.Server{Addr: agent.listenAddr, Net: "udp"}
	tcpServer := &dns.Server{Addr: agent.listenAddr, Net: "tcp"}

	go func() {
		if err := udpServer.ListenAndServe(); err != nil {
			log.Fatalf("[ERRO] Falha ao iniciar UDP local: %v (Dica: execute como Administrador/root para usar a porta 53)", err)
		}
	}()

	go func() {
		if err := tcpServer.ListenAndServe(); err != nil {
			log.Printf("[AVISO] TCP local não disponível: %v", err)
		}
	}()

	log.Println("[INFO] Pronto! Aponte o DNS do seu computador para 127.0.0.1 para navegar protegido.")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("\n[INFO] Encerrando CoorpDNS Agent...")
	udpServer.Shutdown()
	tcpServer.Shutdown()
}

func (a *Agent) handleDNS(w dns.ResponseWriter, req *dns.Msg) {
	if len(req.Question) == 0 {
		dns.HandleFailed(w, req)
		return
	}

	q := req.Question[0]
	qname := strings.ToLower(q.Name)

	// 1. Encaminha via DoH autenticado para a nuvem
	resp, err := a.queryCloudDoH(req)
	if err == nil && resp != nil {
		resp.Id = req.Id
		w.WriteMsg(resp)

		isSinkhole := false
		if len(resp.Answer) > 0 {
			if aRecord, ok := resp.Answer[0].(*dns.A); ok && aRecord.A.String() == "0.0.0.0" {
				isSinkhole = true
			}
		}

		if isSinkhole {
			log.Printf("[BLOQUEADO] %s -> 0.0.0.0 (Sinkhole)", qname)
		} else {
			log.Printf("[OK] %s (Nuvem DoH)", qname)
		}
		return
	}

	// 2. Se a nuvem estiver inatingível, usa fallback local
	log.Printf("[FALLBACK] Falha na nuvem (%v). Consultando fallback %s para %s", err, a.fallback, qname)
	fbResp, _, fbErr := a.udpClient.Exchange(req, a.fallback)
	if fbErr == nil && fbResp != nil {
		fbResp.Id = req.Id
		w.WriteMsg(fbResp)
		return
	}

	dns.HandleFailed(w, req)
}

func (a *Agent) queryCloudDoH(req *dns.Msg) (*dns.Msg, error) {
	packed, err := req.Pack()
	if err != nil {
		return nil, err
	}

	targetURL := fmt.Sprintf("%s/dns-query/%s", a.serverURL, a.token)
	httpReq, err := http.NewRequest("POST", targetURL, bytes.NewReader(packed))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/dns-message")
	httpReq.Header.Set("Accept", "application/dns-message")

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	dnsResp := new(dns.Msg)
	if err := dnsResp.Unpack(body); err != nil {
		return nil, err
	}
	return dnsResp, nil
}
