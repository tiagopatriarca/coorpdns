package dns

import (
	"net"
	"strings"
	"sync"
	"time"

	"github.com/coorpdns/coorpdns/internal/config"
	"github.com/coorpdns/coorpdns/internal/filter"
	"github.com/coorpdns/coorpdns/internal/storage"
	"github.com/miekg/dns"
)

type Server struct {
	cfg        *config.Config
	filter     *filter.Engine
	storage    *storage.DB
	cache      *Cache
	upstream   *UpstreamResolver
	udpServer  *dns.Server
	tcpServer  *dns.Server
	mu         sync.Mutex
	isRunning  bool
}

func NewServer(cfg *config.Config, filter *filter.Engine, store *storage.DB) *Server {
	return &Server{
		cfg:      cfg,
		filter:   filter,
		storage:  store,
		cache:    NewCache(),
		upstream: NewUpstreamResolver(cfg.Upstreams),
	}
}

func (s *Server) ReloadConditionalForwarders() {
	forwarders, err := s.storage.GetConditionalForwarders()
	if err != nil {
		return
	}
	m := make(map[string]string)
	for _, f := range forwarders {
		if f.Enabled {
			m[f.Domain] = f.UpstreamIP
		}
	}
	s.upstream.SetConditionalForwarders(m)
}

func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ReloadConditionalForwarders()

	dns.HandleFunc(".", s.handleDNSRequest)

	s.udpServer = &dns.Server{Addr: s.cfg.Server.DNSAddress, Net: "udp"}
	s.tcpServer = &dns.Server{Addr: s.cfg.Server.DNSAddress, Net: "tcp"}

	errChan := make(chan error, 2)

	go func() {
		if err := s.udpServer.ListenAndServe(); err != nil {
			errChan <- err
		}
	}()

	go func() {
		if err := s.tcpServer.ListenAndServe(); err != nil {
			errChan <- err
		}
	}()

	s.isRunning = true
	return nil
}

func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.udpServer != nil {
		s.udpServer.Shutdown()
	}
	if s.tcpServer != nil {
		s.tcpServer.Shutdown()
	}
	s.isRunning = false
}

func (s *Server) handleDNSRequest(w dns.ResponseWriter, req *dns.Msg) {
	start := time.Now()

	if len(req.Question) == 0 {
		dns.HandleFailed(w, req)
		return
	}

	q := req.Question[0]
	qname := strings.ToLower(q.Name)
	domain := filter.CleanDomain(qname)
	qtypeStr := dns.TypeToString[q.Qtype]

	// Extrai IP do cliente
	clientIP, _, err := net.SplitHostPort(w.RemoteAddr().String())
	if err != nil {
		clientIP = w.RemoteAddr().String()
	}

	// 1. Checa de forma thread-safe se o bloqueio está ativo globalmente ou em pausa temporária
	isBlocking, _ := s.cfg.IsBlockingEffective()

	// 2. Avalia regras do Sinkhole com categorias configuradas para o grupo do cliente
	blocked := false
	matchedRule := ""

	if isBlocking {
		groupID := s.storage.GetClientGroupID(clientIP)
		domainRules, _ := s.storage.GetGroupDomainRulesMap(groupID)
		appRules, _ := s.storage.GetGroupApplicationRules(groupID)
		catRules, _ := s.storage.GetGroupCategoryRules(groupID)
		sched, _ := s.storage.GetGroupSchedule(groupID)

		schedEnabled := sched.IsActive()
		schedMode := ""
		schedInSlot := false
		if schedEnabled {
			schedMode = sched.Mode
			schedInSlot = sched.IsInSchedule(time.Now())
		}

		blocked, matchedRule = s.filter.CheckPolicy(domain, domainRules, appRules, catRules, schedEnabled, schedMode, schedInSlot)
	}

	if blocked {
		resp := s.buildSinkholeResponse(req, q, matchedRule)
		w.WriteMsg(resp)

		latency := float64(time.Since(start).Microseconds()) / 1000.0
		s.storage.LogQuery(&storage.QueryLogRecord{
			Timestamp:   time.Now(),
			ClientIP:    clientIP,
			Domain:      domain,
			QType:       qtypeStr,
			Status:      "blocked",
			RuleMatched: matchedRule,
			Upstream:    "sinkhole",
			LatencyMs:   latency,
		})
		return
	}

	// 3. Checa cache em memória
	if cached := s.cache.Get(qname, q.Qtype); cached != nil {
		cached.Id = req.Id
		w.WriteMsg(cached)

		latency := float64(time.Since(start).Microseconds()) / 1000.0
		s.storage.LogQuery(&storage.QueryLogRecord{
			Timestamp:   time.Now(),
			ClientIP:    clientIP,
			Domain:      domain,
			QType:       qtypeStr,
			Status:      "allowed",
			RuleMatched: "",
			Upstream:    "cache",
			LatencyMs:   latency,
		})
		return
	}

	// 4. Resolve via Upstream
	resp, upstreamUsed, err := s.upstream.Resolve(req)
	if err != nil || resp == nil {
		dns.HandleFailed(w, req)
		return
	}

	// Salva no cache se tiver respostas
	s.cache.Set(qname, q.Qtype, resp)

	resp.Id = req.Id
	w.WriteMsg(resp)

	latency := float64(time.Since(start).Microseconds()) / 1000.0
	s.storage.LogQuery(&storage.QueryLogRecord{
		Timestamp:   time.Now(),
		ClientIP:    clientIP,
		Domain:      domain,
		QType:       qtypeStr,
		Status:      "allowed",
		RuleMatched: "",
		Upstream:    upstreamUsed,
		LatencyMs:   latency,
	})
}

// Cria resposta para domínios bloqueados (0.0.0.0 ou IP do servidor para página de alerta)
func (s *Server) buildSinkholeResponse(req *dns.Msg, q dns.Question, matchedRule string) *dns.Msg {
	resp := new(dns.Msg)
	resp.SetReply(req)
	resp.Authoritative = true
	resp.Rcode = dns.RcodeSuccess

	ttl := s.cfg.GetBlockTTL()

	header := dns.RR_Header{
		Name:   q.Name,
		Rrtype: q.Qtype,
		Class:  dns.ClassINET,
		Ttl:    ttl,
	}

	targetIPv4 := net.ParseIP("0.0.0.0")
	blockMode, serverIP, _, _, _, _ := s.cfg.GetSupportInfo()
	// Se modo de página de bloqueio estiver ativo e tiver IPv4 válido configurado, direciona para o servidor
	if blockMode == "block_page" && serverIP != "" {
		serverHost := serverIP
		serverHost = strings.TrimPrefix(serverHost, "https://")
		serverHost = strings.TrimPrefix(serverHost, "http://")
		if idx := strings.Index(serverHost, "/"); idx != -1 {
			serverHost = serverHost[:idx]
		}
		if idx := strings.Index(serverHost, ":"); idx != -1 {
			serverHost = serverHost[:idx]
		}
		if parsed := net.ParseIP(serverHost); parsed != nil && parsed.To4() != nil {
			targetIPv4 = parsed.To4()
		}
	}

	switch q.Qtype {
	case dns.TypeA:
		resp.Answer = append(resp.Answer, &dns.A{
			Hdr: header,
			A:   targetIPv4,
		})
	case dns.TypeAAAA:
		resp.Answer = append(resp.Answer, &dns.AAAA{
			Hdr:  header,
			AAAA: net.ParseIP("::"),
		})
	default:
		// Para tipos como HTTPS, TXT, etc., apenas retorna NOERROR com lista vazia
	}

	return resp
}

