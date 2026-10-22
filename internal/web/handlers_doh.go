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

// handleDoH processa consultas DNS-over-HTTPS autenticadas por token exclusivo: /dns-query/{token}
func (s *Server) handleDoH(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// 1. Extrai token da URL (/dns-query/{token}) ou do cabeçalho/query param
	path := strings.TrimPrefix(r.URL.Path, "/dns-query")
	token := strings.Trim(path, "/")
	if token == "" {
		token = r.URL.Query().Get("token")
	}

	if token == "" {
		http.Error(w, "CoorpDNS: Autenticação obrigatória. Forneça o token do seu dispositivo na URL (/dns-query/SEU_TOKEN).", http.StatusUnauthorized)
		return
	}

	// 2. Valida cliente e status no banco de dados
	client, err := s.storage.GetClientByToken(token)
	if err != nil || client == nil || client.Status != "active" {
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
		if dnsParam == "" {
			http.Error(w, "Parâmetro ?dns obrigatório", http.StatusBadRequest)
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

	// 4. Checagem de Bloqueio / Sinkhole com categorias por grupo
	isBlocking := s.cfg.IsBlockingActive() && client.BlockEnabled
	blocked := false
	matchedRule := ""

	if isBlocking {
		appRules, _ := s.storage.GetGroupApplicationRules(client.GroupID)
		catRules, _ := s.storage.GetGroupCategoryRules(client.GroupID)
		sched, _ := s.storage.GetGroupSchedule(client.GroupID)

		schedEnabled := sched != nil && sched.Enabled
		schedMode := ""
		schedInSlot := false
		if schedEnabled {
			schedMode = sched.Mode
			schedInSlot = sched.IsInSchedule(time.Now())
		}

		blocked, matchedRule = s.filter.CheckPolicy(domain, appRules, catRules, schedEnabled, schedMode, schedInSlot)
	}

	var respMsg *dns.Msg
	upstreamUsed := "sinkhole"

	if blocked {
		respMsg = new(dns.Msg)
		respMsg.SetReply(reqMsg)
		respMsg.Authoritative = true
		respMsg.Rcode = dns.RcodeSuccess

		ttl := uint32(s.cfg.Server.BlockTTL)
		if ttl == 0 {
			ttl = 10
		}

		header := dns.RR_Header{
			Name:   q.Name,
			Rrtype: q.Qtype,
			Class:  dns.ClassINET,
			Ttl:    ttl,
		}

		targetIPv4 := net.ParseIP("0.0.0.0")
		if s.cfg.Server.BlockMode == "block_page" {
			serverHost := s.cfg.Server.ServerIP
			serverHost = strings.TrimPrefix(serverHost, "https://")
			serverHost = strings.TrimPrefix(serverHost, "http://")
			if idx := strings.Index(serverHost, "/"); idx != -1 {
				serverHost = serverHost[:idx]
			}
			if idx := strings.Index(serverHost, ":"); idx != -1 {
				serverHost = serverHost[:idx]
			}
			if parsed := net.ParseIP(serverHost); parsed != nil {
				targetIPv4 = parsed
			} else {
				targetIPv4 = net.ParseIP("187.77.46.13")
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

	// Usa o IP virtual ou identificador do cliente remoto para que as consultas apareçam sob o nome da máquina
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
