package web

import (
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/coorpdns/coorpdns/internal/filter"
	"github.com/coorpdns/coorpdns/internal/storage"
	"github.com/miekg/dns"
)

// handleDoH processa consultas DNS-over-HTTPS autenticadas por token exclusivo via Bearer header ou URL (/dns-query/{token})
func (s *Server) handleDoH(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	clientIP := s.clientIP(r)
	dohKey := "doh:" + clientIP
	if s.tokenLimiter != nil && !s.tokenLimiter.AllowWithLimit(dohKey, 20, 5*time.Minute) {
		http.Error(w, "CoorpDNS: Muitas falhas de autenticação DoH. Tente novamente mais tarde.", http.StatusTooManyRequests)
		return
	}

	// 1. Extrai token preferencialmente de cabeçalho HTTP (evitando logs em proxy reverso), com fallback para URL
	token := ""
	if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
		token = strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	}
	if token == "" {
		token = strings.TrimSpace(r.Header.Get("X-CoorpDNS-Token"))
	}
	if token == "" {
		path := strings.TrimPrefix(r.URL.Path, "/dns-query")
		token = strings.Trim(path, "/")
	}
	if token == "" {
		token = strings.TrimSpace(r.URL.Query().Get("token"))
	}

	if token == "" {
		http.Error(w, "CoorpDNS: Autenticação obrigatória. Envie Authorization: Bearer <TOKEN> ou /dns-query/<TOKEN>.", http.StatusUnauthorized)
		return
	}

	// 2. Valida formato e status do cliente no banco de dados
	if !tokenRegex.MatchString(token) {
		if s.tokenLimiter != nil {
			s.tokenLimiter.RecordFailedAttempt(dohKey)
		}
		http.Error(w, "CoorpDNS: Formato de token inválido.", http.StatusForbidden)
		return
	}

	client, err := s.storage.GetClientByToken(token)
	if err != nil || client == nil || client.Status != "active" {
		if s.tokenLimiter != nil {
			s.tokenLimiter.RecordFailedAttempt(dohKey)
		}
		http.Error(w, "CoorpDNS: Cliente não autorizado ou token revogado.", http.StatusForbidden)
		return
	}

	// 3. Lê o payload DNS (POST RFC 8484 ou GET com base64url)
	var dnsPayload []byte

	if r.Method == http.MethodPost {
		// Proteção DoS: Pacotes DNS legítimos não excedem 64KB
		dnsPayload, err = io.ReadAll(io.LimitReader(r.Body, 65536))
		if err != nil || len(dnsPayload) == 0 {
			http.Error(w, "Corpo DNS inválido ou excessivo", http.StatusBadRequest)
			return
		}
	} else if r.Method == http.MethodGet {
		dnsParam := r.URL.Query().Get("dns")
		if dnsParam == "" || len(dnsParam) > 88000 {
			http.Error(w, "Parâmetro ?dns obrigatório ou excessivo", http.StatusBadRequest)
			return
		}
		dnsPayload, err = base64.RawURLEncoding.DecodeString(dnsParam)
		if err != nil {
			http.Error(w, "Decodificação base64url falhou", http.StatusBadRequest)
			return
		}
	} else {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	reqMsg := new(dns.Msg)
	if err := reqMsg.Unpack(dnsPayload); err != nil {
		http.Error(w, "Erro ao desempacotar mensagem DNS", http.StatusBadRequest)
		return
	}

	if len(reqMsg.Question) == 0 {
		http.Error(w, "Consulta sem perguntas", http.StatusBadRequest)
		return
	}

	q := reqMsg.Question[0]
	qname := strings.ToLower(q.Name)
	domain := filter.CleanDomain(qname)
	qtypeStr := dns.TypeToString[q.Qtype]

	// 4. Checagem de Bloqueio / Sinkhole respeitando pausa temporária e categorias por grupo
	globalActive, _ := s.cfg.IsBlockingEffective()
	isBlocking := globalActive && client.BlockEnabled
	blocked := false
	matchedRule := ""

	if isBlocking {
		domainRules, _ := s.storage.GetGroupDomainRulesMap(client.GroupID)
		appRules, _ := s.storage.GetGroupApplicationRules(client.GroupID)
		catRules, _ := s.storage.GetGroupCategoryRules(client.GroupID)
		sched, _ := s.storage.GetGroupSchedule(client.GroupID)

		schedEnabled := sched.IsActive()
		schedMode := ""
		schedInSlot := false
		if schedEnabled {
			schedMode = sched.Mode
			schedInSlot = sched.IsInSchedule(time.Now())
		}

		blocked, matchedRule = s.filter.CheckPolicy(domain, domainRules, appRules, catRules, schedEnabled, schedMode, schedInSlot)
	}

	var respMsg *dns.Msg
	upstreamUsed := "sinkhole"

	if blocked {
		respMsg = new(dns.Msg)
		respMsg.SetReply(reqMsg)
		respMsg.Authoritative = true
		respMsg.Rcode = dns.RcodeSuccess

		ttl := s.cfg.GetBlockTTL()
		header := dns.RR_Header{
			Name:   q.Name,
			Rrtype: q.Qtype,
			Class:  dns.ClassINET,
			Ttl:    ttl,
		}

		targetIPv4 := net.ParseIP("0.0.0.0")
		blockMode, serverIP, _, _, _, _ := s.cfg.GetSupportInfo()
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

		if q.Qtype == dns.TypeA {
			respMsg.Answer = append(respMsg.Answer, &dns.A{Hdr: header, A: targetIPv4})
		} else if q.Qtype == dns.TypeAAAA {
			respMsg.Answer = append(respMsg.Answer, &dns.AAAA{Hdr: header, AAAA: net.ParseIP("::")})
		}
	} else {
		// Resolve via Upstream Seguro
		resp, up, err := s.upstream.Resolve(reqMsg)
		if err != nil || resp == nil {
			http.Error(w, "Falha na resolução upstream", http.StatusBadGateway)
			return
		}
		respMsg = resp
		upstreamUsed = up
	}

	// 5. Empacota resposta DNS binária
	packed, err := respMsg.Pack()
	if err != nil {
		http.Error(w, "Erro ao empacotar resposta", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/dns-message")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	w.Write(packed)

	// 6. Registra consulta associada ao Cliente Remoto para os Relatórios de 180 Dias
	latency := float64(time.Since(start).Microseconds()) / 1000.0
	statusStr := "allowed"
	if blocked {
		statusStr = "blocked"
	}

	clientIdentifier := client.IP
	if clientIdentifier == "" {
		clientIdentifier = client.Name
	}

	s.storage.LogQuery(&storage.QueryLogRecord{
		Timestamp:   time.Now(),
		ClientIP:    clientIdentifier,
		Domain:      domain,
		QType:       qtypeStr,
		Status:      statusStr,
		RuleMatched: matchedRule,
		Upstream:    upstreamUsed,
		LatencyMs:   latency,
	})
}
