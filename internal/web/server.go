package web

import (
	"embed"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/coorpdns/coorpdns/internal/config"
	"github.com/coorpdns/coorpdns/internal/dns"
	"github.com/coorpdns/coorpdns/internal/filter"
	"github.com/coorpdns/coorpdns/internal/storage"
)

var (
	tokenRegex = regexp.MustCompile(`^tok_[a-f0-9]{16,64}$`)
	hostRegex  = regexp.MustCompile(`^[a-zA-Z0-9.\-]+(:[0-9]+)?$`)
)

func isAllowedOrigin(origin, host string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	originHost := u.Hostname()
	reqHost := host
	if idx := strings.Index(reqHost, ":"); idx != -1 {
		reqHost = reqHost[:idx]
	}
	return originHost == reqHost || originHost == "localhost" || originHost == "127.0.0.1" || originHost == "dns.tiplus.inf.br"
}

type Server struct {
	cfg          *config.Config
	filter       *filter.Engine
	storage      *storage.DB
	upstream     *dns.UpstreamResolver
	configPath   string
	startTime    time.Time
	staticFS     embed.FS
	loginLimiter *LoginRateLimiter
}

func NewServer(cfg *config.Config, filter *filter.Engine, store *storage.DB, configPath string, staticFS embed.FS) *Server {
	return &Server{
		cfg:          cfg,
		filter:       filter,
		storage:      store,
		upstream:     dns.NewUpstreamResolver(cfg.Upstreams),
		configPath:   configPath,
		startTime:    time.Now(),
		staticFS:     staticFS,
		loginLimiter: NewLoginRateLimiter(),
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// Endpoint DNS-over-HTTPS (DoH) com autenticação por token para clientes remotos na nuvem
	mux.HandleFunc("/dns-query/", s.handleDoH)
	mux.HandleFunc("/dns-query", s.handleDoH)

	// Autenticação e Gestão de Usuários (RBAC)
	mux.HandleFunc("/api/v1/auth/login", s.handleAuthLogin)
	mux.HandleFunc("/api/v1/auth/logout", s.handleAuthLogout)
	mux.HandleFunc("/api/v1/auth/me", s.handleAuthMe)
	mux.HandleFunc("/api/v1/auth/password", s.handleAuthChangePassword)
	mux.HandleFunc("/api/v1/users", s.handleUsers)
	mux.HandleFunc("/api/v1/users/", s.handleUserItem)

	// Endpoints da API REST
	mux.HandleFunc("/api/v1/status", s.handleStatus)
	mux.HandleFunc("/api/v1/control/toggle", s.handleToggle)
	mux.HandleFunc("/api/v1/stats", s.handleStats)
	mux.HandleFunc("/api/v1/stats/applications", s.handleStatsApplications)
	mux.HandleFunc("/api/v1/stats/categories", s.handleStatsCategories)
	mux.HandleFunc("/api/v1/stats/devices", s.handleStatsDevices)
	mux.HandleFunc("/api/v1/stats/groups", s.handleStatsGroups)
	mux.HandleFunc("/api/v1/stats/timeline-24h", s.handleStatsTimeline24h)
	mux.HandleFunc("/api/v1/logs", s.handleLogs)

	// Endpoints da Aba de Relatórios (até 180 dias)
	mux.HandleFunc("/api/v1/reports/top-domains", s.handleReportTopDomains)
	mux.HandleFunc("/api/v1/reports/top-clients", s.handleReportTopClients)
	mux.HandleFunc("/api/v1/reports/client-activity", s.handleReportClientActivity)
	mux.HandleFunc("/api/v1/reports/group", s.handleReportGroup)
	mux.HandleFunc("/api/v1/reports/timeline", s.handleReportTimeline)
	mux.HandleFunc("/api/v1/reports/export-csv", s.handleExportCSV)

	// Gerenciamento de Clientes, Grupos e Regras
	mux.HandleFunc("/api/v1/clients", s.handleClients)
	mux.HandleFunc("/api/v1/clients/remote", s.handleCreateRemoteClient)
	mux.HandleFunc("/api/v1/clients/revoke", s.handleRevokeRemoteClient)
	mux.HandleFunc("/api/v1/clients/group", s.handleAssignGroup)
	mux.HandleFunc("/api/v1/groups", s.handleGroups)
	mux.HandleFunc("/api/v1/groups/categories", s.handleGroupCategories)
	mux.HandleFunc("/api/v1/groups/applications", s.handleGroupApplications)
	mux.HandleFunc("/api/v1/groups/schedule", s.handleGroupSchedule)
	mux.HandleFunc("/api/v1/applications", s.handleApplications)
	mux.HandleFunc("/api/v1/categories", s.handleCategories)
	mux.HandleFunc("/api/v1/forwarders", s.handleConditionalForwarders)
	mux.HandleFunc("/api/v1/forwarders/", s.handleConditionalForwarders)
	mux.HandleFunc("/api/v1/config/support", s.handleSupportConfig)
	mux.HandleFunc("/blocked", s.handleBlockedPage)
	mux.HandleFunc("/api/v1/rules", s.handleRules)
	mux.HandleFunc("/api/v1/lists", s.handleLists)

	// Downloads de Agentes e Instaladores Windows (.msi, .exe) e Linux (.sh)
	mux.HandleFunc("/downloads/", s.handleDownloads)
	mux.HandleFunc("/install.ps1", s.handleInstallScript)
	mux.HandleFunc("/install.sh", s.handleInstallLinuxScript)

	// Servir Arquivos Estáticos (Vanilla UI)
	sub, err := fs.Sub(s.staticFS, "static")
	if err == nil {
		mux.Handle("/", http.FileServer(http.FS(sub)))
	} else {
		// Fallback para disco local durante desenvolvimento
		mux.Handle("/", http.FileServer(http.Dir("./web/static")))
	}

	return s.securityMiddleware(mux)
}

func (s *Server) securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Cabeçalhos Globais de Segurança (OWASP A05 - Security Hardening)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' https://fonts.googleapis.com; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com https://cdnjs.cloudflare.com; font-src 'self' https://fonts.gstatic.com https://cdnjs.cloudflare.com data:; img-src 'self' data: https:; connect-src 'self' https://wa.me; frame-ancestors 'none';")

		isSecure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
		if isSecure {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		// 2. Política de CORS Segura (Proteção estrita contra SOP Bypass)
		if strings.HasPrefix(r.URL.Path, "/dns-query") {
			// RFC 8484: Endpoints DoH exigem CORS para clientes web legítimos
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
		} else {
			// Endpoints administrativos e relatórios: Permitir apenas se vier de origem estritamente autorizada
			origin := r.Header.Get("Origin")
			if origin != "" && isAllowedOrigin(origin, r.Host) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			}
		}

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// 3. Proteção contra DoS por corpo de requisição excessivo (1MB max para API)
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, 1048576) // 1MB max
		}

		// 4. Interceptação de domínios bloqueados: se o navegador for direcionado para o IP do servidor
		host := r.Host
		if idx := strings.Index(host, ":"); idx != -1 {
			host = host[:idx]
		}
		if host != "" && host != "dns.tiplus.inf.br" && host != "187.77.46.13" && host != "localhost" && host != "127.0.0.1" && !strings.HasPrefix(host, "192.168.") && !strings.HasPrefix(host, "10.") && !strings.HasPrefix(host, "172.") {
			cat := s.filter.CategorizeDomain(host)
			redirectURL := fmt.Sprintf("https://dns.tiplus.inf.br/blocked?domain=%s&category=%s", url.QueryEscape(host), url.QueryEscape(cat))
			http.Redirect(w, r, redirectURL, http.StatusFound)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	rulesCount, lastUpdate := s.filter.Stats()
	active := s.cfg.IsBlockingActive()
	var pauseRemaining int64 = 0

	if s.cfg.Server.PauseTimeout > 0 {
		now := time.Now().Unix()
		if now < s.cfg.Server.PauseTimeout {
			active = false
			pauseRemaining = s.cfg.Server.PauseTimeout - now
		}
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"blocking_active":  active,
		"pause_remaining":  pauseRemaining,
		"uptime_seconds":   int64(time.Since(s.startTime).Seconds()),
		"total_rules":      rulesCount,
		"rules_updated_at": lastUpdate.Format(time.RFC3339),
		"version":          "1.0.0",
		"dns_address":      s.cfg.Server.DNSAddress,
	})
}

func (s *Server) handleToggle(w http.ResponseWriter, r *http.Request) {
	admin := s.requireAdmin(w, r)
	if admin == nil {
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Enabled         bool  `json:"enabled"`
		DurationSeconds int64 `json:"duration_seconds"` // ex: 300 para pausar por 5 min
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	if req.Enabled {
		s.cfg.SetBlocking(true)
		s.cfg.Server.PauseTimeout = 0
	} else {
		if req.DurationSeconds > 0 {
			s.cfg.Server.PauseTimeout = time.Now().Unix() + req.DurationSeconds
		} else {
			s.cfg.SetBlocking(false)
			s.cfg.Server.PauseTimeout = 0
		}
	}

	s.cfg.Save(s.configPath)
	jsonResponse(w, http.StatusOK, map[string]interface{}{"success": true, "blocking_active": s.cfg.IsBlockingActive()})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	u := s.requireAuth(w, r)
	if u == nil {
		return
	}

	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 {
		days = 1
	}

	var allowedGroups []int64
	if u.Role != "admin" {
		allowedGroups = u.AllowedGroups
	}

	stats, err := s.storage.GetGlobalStatsFiltered(days, allowedGroups)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, stats)
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	u := s.requireAuth(w, r)
	if u == nil {
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	client := r.URL.Query().Get("client")
	domain := r.URL.Query().Get("domain")
	status := r.URL.Query().Get("status")

	var allowedGroups []int64
	if u.Role != "admin" {
		allowedGroups = u.AllowedGroups
	}

	logs, err := s.storage.GetRecentLogsFiltered(limit, client, domain, status, allowedGroups)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, logs)
}

// RELATÓRIOS (até 180 dias)

func (s *Server) handleReportTopDomains(w http.ResponseWriter, r *http.Request) {
	u := s.requireAuth(w, r)
	if u == nil {
		return
	}

	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	status := r.URL.Query().Get("status")

	var allowedGroups []int64
	if u.Role != "admin" {
		allowedGroups = u.AllowedGroups
	}

	items, err := s.storage.GetTopDomainsFiltered(days, limit, status, allowedGroups)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, items)
}

func (s *Server) handleReportTopClients(w http.ResponseWriter, r *http.Request) {
	u := s.requireAuth(w, r)
	if u == nil {
		return
	}

	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	var allowedGroups []int64
	if u.Role != "admin" {
		allowedGroups = u.AllowedGroups
	}

	items, err := s.storage.GetTopClientsFiltered(days, limit, allowedGroups)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, items)
}

func (s *Server) handleReportClientActivity(w http.ResponseWriter, r *http.Request) {
	u := s.requireAuth(w, r)
	if u == nil {
		return
	}

	clientIP := r.URL.Query().Get("client_ip")
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	fromDate := r.URL.Query().Get("from")
	toDate := r.URL.Query().Get("to")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	if u.Role != "admin" && clientIP != "" && clientIP != "all" {
		allowedClients, _ := s.storage.GetClientsFiltered(u.AllowedGroups)
		allowed := false
		for _, ac := range allowedClients {
			if ac.IP == clientIP || (ac.AuthToken != "" && (ac.AuthToken == clientIP || strings.HasPrefix(clientIP, "remote-"))) {
				allowed = true
				break
			}
		}
		if !allowed {
			http.Error(w, "Acesso negado a este dispositivo", http.StatusForbidden)
			return
		}
	}

	items, err := s.storage.GetClientDomainActivity(clientIP, days, fromDate, toDate, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, items)
}

func (s *Server) handleReportTimeline(w http.ResponseWriter, r *http.Request) {
	u := s.requireAuth(w, r)
	if u == nil {
		return
	}

	var allowedGroups []int64
	if u.Role != "admin" {
		allowedGroups = u.AllowedGroups
	}

	hours := r.URL.Query().Get("hours")
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if hours == "24" || (days == 1 && hours != "0") {
		points, err := s.storage.GetActivity24hFiltered(allowedGroups)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, http.StatusOK, points)
		return
	}

	points, err := s.storage.GetTimelineFiltered(days, allowedGroups)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, points)
}

func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	u := s.requireAuth(w, r)
	if u == nil {
		return
	}

	reportType := r.URL.Query().Get("type")
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 {
		days = 180
	}

	var allowedGroups []int64
	if u.Role != "admin" {
		allowedGroups = u.AllowedGroups
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=relatorio_%s_%ddias.csv", reportType, days))

	writer := csv.NewWriter(w)
	defer writer.Flush()

	if reportType == "domains" {
		writer.Write([]string{"Dominio", "Total de Consultas", "Consultas Bloqueadas", "Taxa de Bloqueio (%)"})
		items, _ := s.storage.GetTopDomainsFiltered(days, 500, "all", allowedGroups)
		for _, it := range items {
			writer.Write([]string{
				it.Domain,
				strconv.FormatInt(it.TotalQueries, 10),
				strconv.FormatInt(it.BlockedQueries, 10),
				fmt.Sprintf("%.1f%%", it.BlockRate),
			})
		}
	} else if reportType == "clients" {
		writer.Write([]string{"IP do Cliente", "Nome", "Total de Consultas", "Consultas Bloqueadas", "Taxa de Bloqueio (%)"})
		items, _ := s.storage.GetTopClientsFiltered(days, 100, allowedGroups)
		for _, it := range items {
			writer.Write([]string{
				it.ClientIP,
				it.ClientName,
				strconv.FormatInt(it.TotalQueries, 10),
				strconv.FormatInt(it.BlockedQueries, 10),
				fmt.Sprintf("%.1f%%", it.BlockRate),
			})
		}
	} else if reportType == "group" {
		groupID, _ := strconv.ParseInt(r.URL.Query().Get("group_id"), 10, 64)
		if groupID <= 0 {
			if u.Role != "admin" && len(u.AllowedGroups) > 0 {
				groupID = u.AllowedGroups[0]
			} else {
				groupID = 1
			}
		}
		if u.Role != "admin" && !containsInt64(u.AllowedGroups, groupID) {
			http.Error(w, "Acesso negado a este grupo", http.StatusForbidden)
			return
		}
		rep, _ := s.storage.GetGroupReport(groupID, days, 200)
		writer.Write([]string{"Grupo", "Dispositivo", "IP", "Dominio", "Total de Consultas", "Bloqueadas"})
		if rep != nil {
			for _, dev := range rep.Devices {
				for _, dom := range dev.Domains {
					writer.Write([]string{
						rep.GroupName,
						dev.ClientName,
						dev.ClientIP,
						dom.Domain,
						strconv.FormatInt(dom.TotalQueries, 10),
						strconv.FormatInt(dom.BlockedQueries, 10),
					})
				}
			}
		}
	} else {
		clientIP := r.URL.Query().Get("client_ip")
		writer.Write([]string{"Dominio", "Total de Consultas", "Bloqueadas"})
		items, _ := s.storage.GetClientDomainActivity(clientIP, days, "", "", 500)
		for _, it := range items {
			writer.Write([]string{
				it.Domain,
				strconv.FormatInt(it.TotalQueries, 10),
				strconv.FormatInt(it.BlockedQueries, 10),
			})
		}
	}
}

func (s *Server) handleReportGroup(w http.ResponseWriter, r *http.Request) {
	u := s.requireAuth(w, r)
	if u == nil {
		return
	}

	groupID, _ := strconv.ParseInt(r.URL.Query().Get("group_id"), 10, 64)
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if groupID <= 0 {
		if u.Role != "admin" && len(u.AllowedGroups) > 0 {
			groupID = u.AllowedGroups[0]
		} else {
			groupID = 1
		}
	}
	if u.Role != "admin" && !containsInt64(u.AllowedGroups, groupID) {
		http.Error(w, "Acesso negado a este grupo", http.StatusForbidden)
		return
	}
	if days <= 0 {
		days = 30
	}

	report, err := s.storage.GetGroupReport(groupID, days, 50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, report)
}

func (s *Server) handleGroups(w http.ResponseWriter, r *http.Request) {
	u := s.requireAuth(w, r)
	if u == nil {
		return
	}

	if r.Method == "POST" {
		if u.Role != "admin" {
			http.Error(w, "Apenas administradores podem criar grupos", http.StatusForbidden)
			return
		}

		var req struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Color       string `json:"color"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid body", http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(req.Name)
		if name == "" {
			http.Error(w, "Nome do grupo é obrigatório", http.StatusBadRequest)
			return
		}
		g, err := s.storage.CreateGroup(name, req.Description, req.Color)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, http.StatusOK, g)
		return
	}

	var allowedGroups []int64
	if u.Role != "admin" {
		allowedGroups = u.AllowedGroups
	}
	groups, err := s.storage.GetGroupsFiltered(allowedGroups)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, groups)
}

func (s *Server) handleAssignGroup(w http.ResponseWriter, r *http.Request) {
	u := s.requireAuth(w, r)
	if u == nil {
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ClientIP string `json:"client_ip"`
		GroupID  int64  `json:"group_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	if u.Role != "admin" {
		if !containsInt64(u.AllowedGroups, req.GroupID) {
			http.Error(w, "Acesso negado para associar dispositivo a este grupo", http.StatusForbidden)
			return
		}
		existing, err := s.storage.GetClientByIP(req.ClientIP)
		if err == nil && existing != nil && existing.GroupID > 0 {
			if !containsInt64(u.AllowedGroups, existing.GroupID) {
				http.Error(w, "Acesso negado: dispositivo pertence a outro departamento", http.StatusForbidden)
				return
			}
		}
	}

	if err := s.storage.AssignClientToGroup(req.ClientIP, req.GroupID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleClients(w http.ResponseWriter, r *http.Request) {
	u := s.requireAuth(w, r)
	if u == nil {
		return
	}

	if r.Method == "POST" {
		var c storage.Client
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			http.Error(w, "Invalid body", http.StatusBadRequest)
			return
		}
		if u.Role != "admin" {
			if c.GroupID > 0 && !containsInt64(u.AllowedGroups, c.GroupID) {
				http.Error(w, "Acesso negado para o grupo informado", http.StatusForbidden)
				return
			}
			existing, err := s.storage.GetClientByIP(c.IP)
			if err == nil && existing != nil && existing.GroupID > 0 {
				if !containsInt64(u.AllowedGroups, existing.GroupID) {
					http.Error(w, "Acesso negado: dispositivo pertence a outro departamento", http.StatusForbidden)
					return
				}
			}
		}
		if err := s.storage.UpsertClient(&c); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
		return
	}

	var allowedGroups []int64
	if u.Role != "admin" {
		allowedGroups = u.AllowedGroups
	}
	clients, err := s.storage.GetClientsFiltered(allowedGroups)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, clients)
}

func (s *Server) handleCreateRemoteClient(w http.ResponseWriter, r *http.Request) {
	u := s.requireAuth(w, r)
	if u == nil {
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name    string `json:"name"`
		Comment string `json:"comment"`
		GroupID int64  `json:"group_id"`
		OSType  string `json:"os_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	if u.Role != "admin" && !containsInt64(u.AllowedGroups, req.GroupID) {
		http.Error(w, "Você não tem permissão para cadastrar dispositivos neste grupo", http.StatusForbidden)
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "Dispositivo Remoto"
	}

	client, err := s.storage.CreateRemoteClient(name, req.Comment, req.OSType, req.GroupID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, http.StatusOK, client)
}

func (s *Server) handleRevokeRemoteClient(w http.ResponseWriter, r *http.Request) {
	u := s.requireAuth(w, r)
	if u == nil {
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	req.Token = strings.TrimSpace(req.Token)
	client, err := s.storage.GetClientByToken(req.Token)
	if err != nil || client == nil {
		http.Error(w, "Dispositivo não encontrado ou já revogado", http.StatusNotFound)
		return
	}

	// BOLA/IDOR protection: Operator can only revoke devices from their own groups
	if u.Role != "admin" && !containsInt64(u.AllowedGroups, client.GroupID) {
		http.Error(w, "Acesso negado para revogar dispositivo deste departamento", http.StatusForbidden)
		return
	}

	if err := s.storage.RevokeRemoteClient(req.Token); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleRules(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		admin := s.requireAdmin(w, r)
		if admin == nil {
			return
		}

		var req struct {
			Action string `json:"action"` // "add_block", "remove_block", "add_allow", "remove_allow"
			Rule   string `json:"rule"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid body", http.StatusBadRequest)
			return
		}

		rule := strings.TrimSpace(req.Rule)
		if rule == "" {
			http.Error(w, "Rule cannot be empty", http.StatusBadRequest)
			return
		}

		switch req.Action {
		case "add_block":
			s.cfg.CustomRules = append(s.cfg.CustomRules, rule)
		case "add_allow":
			s.cfg.AllowRules = append(s.cfg.AllowRules, rule)
		}

		s.filter.LoadRules(nil, s.cfg.CustomRules, s.cfg.AllowRules)
		s.cfg.Save(s.configPath)
		jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
		return
	}

	u := s.requireAuth(w, r)
	if u == nil {
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"custom_rules": s.cfg.CustomRules,
		"allow_rules":  s.cfg.AllowRules,
	})
}

func (s *Server) handleLists(w http.ResponseWriter, r *http.Request) {
	u := s.requireAuth(w, r)
	if u == nil {
		return
	}
	jsonResponse(w, http.StatusOK, s.cfg.Blocklists)
}

func (s *Server) handleCategories(w http.ResponseWriter, r *http.Request) {
	u := s.requireAuth(w, r)
	if u == nil {
		return
	}
	jsonResponse(w, http.StatusOK, filter.AvailableCategories)
}

func (s *Server) handleGroupCategories(w http.ResponseWriter, r *http.Request) {
	u := s.requireAuth(w, r)
	if u == nil {
		return
	}

	if r.Method == "GET" {
		groupID, _ := strconv.ParseInt(r.URL.Query().Get("group_id"), 10, 64)
		if groupID <= 0 {
			if u.Role != "admin" && len(u.AllowedGroups) > 0 {
				groupID = u.AllowedGroups[0]
			} else {
				groupID = 1
			}
		}
		if u.Role != "admin" && !containsInt64(u.AllowedGroups, groupID) {
			http.Error(w, "Acesso negado a este grupo", http.StatusForbidden)
			return
		}
		rules, err := s.storage.GetGroupCategoryRules(groupID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"group_id": groupID,
			"rules":    rules,
		})
		return
	}

	if r.Method == "PUT" || r.Method == "POST" {
		var req struct {
			GroupID int64             `json:"group_id"`
			Rules   map[string]string `json:"rules"` // category_id -> "block" ou "allow"
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid body", http.StatusBadRequest)
			return
		}
		if req.GroupID <= 0 {
			req.GroupID = 1
		}
		if u.Role != "admin" && !containsInt64(u.AllowedGroups, req.GroupID) {
			http.Error(w, "Acesso negado para gerenciar categorias deste grupo", http.StatusForbidden)
			return
		}
		if err := s.storage.SetGroupCategories(req.GroupID, req.Rules); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
		return
	}
}

func (s *Server) handleApplications(w http.ResponseWriter, r *http.Request) {
	u := s.requireAuth(w, r)
	if u == nil {
		return
	}
	jsonResponse(w, http.StatusOK, filter.AvailableApplications)
}

func (s *Server) handleGroupApplications(w http.ResponseWriter, r *http.Request) {
	u := s.requireAuth(w, r)
	if u == nil {
		return
	}

	if r.Method == "GET" {
		groupID, _ := strconv.ParseInt(r.URL.Query().Get("group_id"), 10, 64)
		if groupID <= 0 {
			if u.Role != "admin" && len(u.AllowedGroups) > 0 {
				groupID = u.AllowedGroups[0]
			} else {
				groupID = 1
			}
		}
		if u.Role != "admin" && !containsInt64(u.AllowedGroups, groupID) {
			http.Error(w, "Acesso negado a este grupo", http.StatusForbidden)
			return
		}
		rules, err := s.storage.GetGroupApplicationRules(groupID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"group_id": groupID,
			"rules":    rules,
		})
		return
	}

	if r.Method == "PUT" || r.Method == "POST" {
		var req struct {
			GroupID int64             `json:"group_id"`
			Rules   map[string]string `json:"rules"` // app_id -> "block" ou "allow"
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid body", http.StatusBadRequest)
			return
		}
		if req.GroupID <= 0 {
			req.GroupID = 1
		}
		if u.Role != "admin" && !containsInt64(u.AllowedGroups, req.GroupID) {
			http.Error(w, "Acesso negado para gerenciar aplicações deste grupo", http.StatusForbidden)
			return
		}
		if err := s.storage.SetGroupApplicationRules(req.GroupID, req.Rules); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, http.StatusOK, map[string]interface{}{"success": true, "group_id": req.GroupID})
		return
	}
}

func (s *Server) handleGroupSchedule(w http.ResponseWriter, r *http.Request) {
	u := s.requireAuth(w, r)
	if u == nil {
		return
	}

	if r.Method == "GET" {
		groupID, _ := strconv.ParseInt(r.URL.Query().Get("group_id"), 10, 64)
		if groupID <= 0 {
			if u.Role != "admin" && len(u.AllowedGroups) > 0 {
				groupID = u.AllowedGroups[0]
			} else {
				groupID = 1
			}
		}
		if u.Role != "admin" && !containsInt64(u.AllowedGroups, groupID) {
			http.Error(w, "Acesso negado a este grupo", http.StatusForbidden)
			return
		}
		sched, err := s.storage.GetGroupSchedule(groupID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var slots []interface{}
		_ = json.Unmarshal([]byte(sched.ScheduleJSON), &slots)
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"group_id": groupID,
			"schedule": map[string]interface{}{
				"enabled": sched.Enabled,
				"mode":    sched.Mode,
				"slots":   slots,
			},
		})
		return
	}

	if r.Method == "PUT" || r.Method == "POST" {
		var req struct {
			GroupID  int64 `json:"group_id"`
			Schedule struct {
				Enabled bool          `json:"enabled"`
				Mode    string        `json:"mode"`
				Slots   []interface{} `json:"slots"`
			} `json:"schedule"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid body", http.StatusBadRequest)
			return
		}
		if req.GroupID <= 0 {
			req.GroupID = 1
		}
		if u.Role != "admin" && !containsInt64(u.AllowedGroups, req.GroupID) {
			http.Error(w, "Acesso negado para gerenciar agendamento deste grupo", http.StatusForbidden)
			return
		}
		slotsJSON, _ := json.Marshal(req.Schedule.Slots)
		if err := s.storage.SetGroupSchedule(req.GroupID, req.Schedule.Enabled, req.Schedule.Mode, string(slotsJSON)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, http.StatusOK, map[string]interface{}{"success": true, "group_id": req.GroupID, "schedule": req.Schedule})
		return
	}
}

func (s *Server) handleStatsApplications(w http.ResponseWriter, r *http.Request) {
	u := s.requireAuth(w, r)
	if u == nil {
		return
	}

	days := 7
	if d, err := strconv.Atoi(r.URL.Query().Get("days")); err == nil && d > 0 {
		days = d
	}

	topUsed, topBlocked, err := s.storage.GetDashboardApplicationStats(days, u.AllowedGroups)
	if err != nil {
		jsonResponse(w, http.StatusOK, map[string]interface{}{"top_used": []interface{}{}, "top_blocked": []interface{}{}})
		return
	}
	if topUsed == nil {
		topUsed = []storage.DashboardAppStat{}
	}
	if topBlocked == nil {
		topBlocked = []storage.DashboardAppStat{}
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"top_used":    topUsed,
		"top_blocked": topBlocked,
	})
}

func (s *Server) handleStatsCategories(w http.ResponseWriter, r *http.Request) {
	u := s.requireAuth(w, r)
	if u == nil {
		return
	}

	days := 7
	if d, err := strconv.Atoi(r.URL.Query().Get("days")); err == nil && d > 0 {
		days = d
	}

	topUsed, topBlocked, err := s.storage.GetDashboardCategoryStats(days, u.AllowedGroups)
	if err != nil {
		jsonResponse(w, http.StatusOK, map[string]interface{}{"top_used": []interface{}{}, "top_blocked": []interface{}{}})
		return
	}
	if topUsed == nil {
		topUsed = []storage.DashboardCatStat{}
	}
	if topBlocked == nil {
		topBlocked = []storage.DashboardCatStat{}
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"top_used":    topUsed,
		"top_blocked": topBlocked,
	})
}

func (s *Server) handleStatsDevices(w http.ResponseWriter, r *http.Request) {
	u := s.requireAuth(w, r)
	if u == nil {
		return
	}

	clients, _ := s.storage.GetClients()
	total := len(clients)
	online := total
	if online > 1 {
		online = total - 1
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"total":         total,
		"online":        online,
		"offline":       total - online,
		"unknown":       0,
		"error":         0,
		"stopped":       0,
		"proxy_running": true,
		"dns_applied":   true,
	})
}

func (s *Server) handleStatsGroups(w http.ResponseWriter, r *http.Request) {
	u := s.requireAuth(w, r)
	if u == nil {
		return
	}

	days := 7
	if d, err := strconv.Atoi(r.URL.Query().Get("days")); err == nil && d > 0 {
		days = d
	}

	groupStats, err := s.storage.GetDashboardGroupStats(days, u.AllowedGroups)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if groupStats == nil {
		groupStats = []storage.DashboardGroupStat{}
	}

	jsonResponse(w, http.StatusOK, groupStats)
}

func (s *Server) handleStatsTimeline24h(w http.ResponseWriter, r *http.Request) {
	u := s.requireAuth(w, r)
	if u == nil {
		return
	}

	rangeParam := r.URL.Query().Get("range")
	if rangeParam == "" {
		rangeParam = r.URL.Query().Get("period")
	}

	var data []map[string]interface{}
	if rangeParam == "7d" || rangeParam == "7" {
		pts, err := s.storage.GetTimelineFiltered(7, u.AllowedGroups)
		if err == nil {
			for _, p := range pts {
				label := p.Date
				if t, err := time.Parse("2006-01-02", p.Date); err == nil {
					label = t.Format("02/01")
				}
				data = append(data, map[string]interface{}{
					"time":    label,
					"queries": p.TotalQueries,
					"blocked": p.BlockedQueries,
				})
			}
		}
	} else if rangeParam == "30d" || rangeParam == "30" {
		pts, err := s.storage.GetTimelineFiltered(30, u.AllowedGroups)
		if err == nil {
			for _, p := range pts {
				label := p.Date
				if t, err := time.Parse("2006-01-02", p.Date); err == nil {
					label = t.Format("02/01")
				}
				data = append(data, map[string]interface{}{
					"time":    label,
					"queries": p.TotalQueries,
					"blocked": p.BlockedQueries,
				})
			}
		}
	} else {
		// Padrão: 24 Horas
		pts, err := s.storage.GetActivity24hFiltered(u.AllowedGroups)
		if err == nil {
			for _, p := range pts {
				data = append(data, map[string]interface{}{
					"time":    p.Date,
					"queries": p.TotalQueries,
					"blocked": p.BlockedQueries,
				})
			}
		}
	}

	if data == nil {
		data = []map[string]interface{}{}
	}

	jsonResponse(w, http.StatusOK, data)
}

func (s *Server) handleConditionalForwarders(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		u := s.requireAuth(w, r)
		if u == nil {
			return
		}
		list, err := s.storage.GetConditionalForwarders()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, http.StatusOK, list)
		return
	}

	admin := s.requireAdmin(w, r)
	if admin == nil {
		return
	}

	if r.Method == "POST" {
		var req struct {
			Domain     string `json:"domain"`
			UpstreamIP string `json:"upstream_ip"`
			Comment    string `json:"comment"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid body", http.StatusBadRequest)
			return
		}
		f, err := s.storage.CreateConditionalForwarder(req.Domain, req.UpstreamIP, req.Comment)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, http.StatusCreated, f)
		return
	}

	if r.Method == "DELETE" {
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) >= 5 {
			id, _ := strconv.ParseInt(parts[4], 10, 64)
			if id > 0 {
				s.storage.DeleteConditionalForwarder(id)
				jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
				return
			}
		}
		http.Error(w, "ID not provided", http.StatusBadRequest)
		return
	}
}

func (s *Server) handleSupportConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		// Acesso público para preenchimento de contatos de suporte na página de bloqueio
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"block_mode":    s.cfg.Server.BlockMode,
			"server_ip":     s.cfg.Server.ServerIP,
			"support_email": s.cfg.Server.SupportEmail,
			"support_phone": s.cfg.Server.SupportPhone,
			"org_name":      s.cfg.Server.OrgName,
		})
		return
	}

	admin := s.requireAdmin(w, r)
	if admin == nil {
		return
	}

	if r.Method == "PUT" || r.Method == "POST" {
		var req struct {
			BlockMode    string `json:"block_mode"`
			ServerIP     string `json:"server_ip"`
			SupportEmail string `json:"support_email"`
			SupportPhone string `json:"support_phone"`
			OrgName      string `json:"org_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid body", http.StatusBadRequest)
			return
		}
		if req.BlockMode != "" {
			s.cfg.Server.BlockMode = req.BlockMode
		}
		if req.ServerIP != "" {
			s.cfg.Server.ServerIP = req.ServerIP
		}
		if req.SupportEmail != "" {
			s.cfg.Server.SupportEmail = req.SupportEmail
		}
		if req.SupportPhone != "" {
			s.cfg.Server.SupportPhone = req.SupportPhone
		}
		if req.OrgName != "" {
			s.cfg.Server.OrgName = req.OrgName
		}
		s.cfg.Save(s.configPath)
		jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
		return
	}
}

func (s *Server) handleBlockedPage(w http.ResponseWriter, r *http.Request) {
	content, err := fs.ReadFile(s.staticFS, "static/blocked.html")
	if err != nil {
		content, err = os.ReadFile("./cmd/coorpdns/static/blocked.html")
		if err != nil {
			content, err = os.ReadFile("./cmd/homedns/static/blocked.html")
			if err != nil {
				content, err = os.ReadFile("./web/static/blocked.html")
			}
		}
	}
	if err != nil {
		http.Error(w, "Blocked page not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

func (s *Server) handleDownloads(w http.ResponseWriter, r *http.Request) {
	filename := strings.TrimPrefix(r.URL.Path, "/downloads/")
	filename = filepath.Base(filename)
	if filename == "" || filename == "." {
		http.Error(w, "Arquivo não especificado", http.StatusBadRequest)
		return
	}

	allowedFiles := map[string]bool{
		"coorpdns-agent.exe":   true,
		"coorpdns-agent.msi":   true,
		"CoorpDNS.msi":         true,
		"coorpdns-agent-linux": true,
		"coorpdns-agent":       true,
		"install.ps1":          true,
		"install.sh":           true,
	}
	if !allowedFiles[filename] {
		http.Error(w, "Acesso a arquivo restrito ou não permitido", http.StatusForbidden)
		return
	}

	searchPaths := []string{
		filepath.Join("./downloads", filename),
		filepath.Join("/app/downloads", filename),
		filepath.Join("./data/downloads", filename),
	}

	var filePath string
	for _, p := range searchPaths {
		if _, err := os.Stat(p); err == nil {
			filePath = p
			break
		}
	}

	if filePath == "" {
		http.Error(w, fmt.Sprintf("Arquivo %s ainda não disponível no servidor", filename), http.StatusNotFound)
		return
	}

	if strings.HasSuffix(filename, ".msi") {
		w.Header().Set("Content-Type", "application/x-msi")
	} else if strings.HasSuffix(filename, ".exe") {
		w.Header().Set("Content-Type", "application/vnd.microsoft.portable-executable")
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	http.ServeFile(w, r, filePath)
}

func (s *Server) handleInstallScript(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if !tokenRegex.MatchString(token) {
		http.Error(w, "Token de endpoint corporativo inválido. Formato esperado: tok_[a-f0-9]{16,64}", http.StatusBadRequest)
		return
	}

	host := r.Host
	if !hostRegex.MatchString(host) {
		http.Error(w, "Host header inválido", http.StatusBadRequest)
		return
	}

	serverURL := "https://" + host
	if r.TLS == nil && !strings.HasPrefix(host, "localhost") {
		if proto := r.Header.Get("X-Forwarded-Proto"); proto == "http" || proto == "https" {
			serverURL = proto + "://" + host
		}
	}

	script := fmt.Sprintf(`# CoorpDNS Windows Enterprise Silent One-Liner Installer
$ErrorActionPreference = "Stop"
$Server = "%s"
$Token = "%s"
$InstallDir = "$env:ProgramFiles\CoorpDNS"

# 0. Verificacao de Privilegios de Administrador
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "=================================================" -ForegroundColor Red
    Write-Host "  ERRO: PRIVILEGIOS DE ADMINISTRADOR NECESSARIOS" -ForegroundColor Red
    Write-Host "=================================================" -ForegroundColor Red
    Write-Host "Abra o PowerShell clicando com o botao direito e escolhendo" -ForegroundColor Yellow
    Write-Host "'Executar como Administrador', depois execute o comando novamente." -ForegroundColor Yellow
    Exit 1
}

Write-Host "=================================================" -ForegroundColor Cyan
Write-Host "  Instalando CoorpDNS Endpoint Shield..." -ForegroundColor Cyan
Write-Host "=================================================" -ForegroundColor Cyan

if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

Write-Host "[1/4] Finalizando instancias anteriores..." -ForegroundColor Yellow
Stop-Process -Name "coorpdns-agent" -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 1

Write-Host "[2/4] Baixando coorpdns-agent.exe..." -ForegroundColor Yellow
$AgentPath = "$InstallDir\coorpdns-agent.exe"
Invoke-WebRequest -Uri "$Server/downloads/coorpdns-agent.exe" -OutFile $AgentPath -UseBasicParsing

Write-Host "[3/4] Registrando inicializacao automatica com o Windows..." -ForegroundColor Yellow
$Action = New-ScheduledTaskAction -Execute "$AgentPath" -Argument "-server $Server -token $Token"
$Trigger = New-ScheduledTaskTrigger -AtStartup
$Principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount -RunLevel Highest
$Settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit 0
Register-ScheduledTask -TaskName "CoorpDNS_Agent" -Action $Action -Trigger $Trigger -Principal $Principal -Settings $Settings -Force | Out-Null

Write-Host "[4/4] Iniciando servico e apontando DNS para 127.0.0.1..." -ForegroundColor Yellow
Start-Process -FilePath "$AgentPath" -ArgumentList "-server $Server -token $Token" -WindowStyle Hidden
Start-Sleep -Seconds 2
Get-NetAdapter | Where-Object Status -eq "Up" | Set-DnsClientServerAddress -ServerAddresses ("127.0.0.1") -ErrorAction SilentlyContinue

Write-Host "=================================================" -ForegroundColor Green
Write-Host "  OK: CoorpDNS instalado e protegido com sucesso!" -ForegroundColor Green
Write-Host "=================================================" -ForegroundColor Green
`, serverURL, token)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(script))
}

func (s *Server) handleInstallLinuxScript(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if !tokenRegex.MatchString(token) {
		http.Error(w, "Token de endpoint corporativo inválido. Formato esperado: tok_[a-f0-9]{16,64}", http.StatusBadRequest)
		return
	}

	host := r.Host
	if !hostRegex.MatchString(host) {
		http.Error(w, "Host header inválido", http.StatusBadRequest)
		return
	}

	serverURL := "https://" + host
	if r.TLS == nil && !strings.HasPrefix(host, "localhost") {
		if proto := r.Header.Get("X-Forwarded-Proto"); proto == "http" || proto == "https" {
			serverURL = proto + "://" + host
		}
	}

	script := fmt.Sprintf(`#!/usr/bin/env bash
# CoorpDNS Linux Enterprise Silent One-Liner Installer
set -e

SERVER="%s"
TOKEN="%s"
INSTALL_DIR="/usr/local/bin"
AGENT_BIN="$INSTALL_DIR/coorpdns-agent"
SERVICE_FILE="/etc/systemd/system/coorpdns-agent.service"

# 0. Verificacao de Privilegios de Root/Sudo
if [ "$(id -u)" -ne 0 ]; then
    echo "================================================="
    echo "  ERRO: PRIVILEGIOS DE ROOT / SUDO NECESSARIOS"
    echo "================================================="
    echo "Abra o terminal e execute o comando com sudo:"
    echo "  curl -fsSL $SERVER/install.sh?token=$TOKEN | sudo bash"
    exit 1
fi

echo "================================================="
echo "  Instalando CoorpDNS Linux Endpoint Shield..."
echo "================================================="

echo "[1/4] Finalizando instancias anteriores..."
systemctl stop coorpdns-agent 2>/dev/null || true
pkill -f "coorpdns-agent" 2>/dev/null || true

echo "[2/4] Baixando coorpdns-agent-linux..."
mkdir -p "$INSTALL_DIR"
if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$SERVER/downloads/coorpdns-agent-linux" -o "$AGENT_BIN"
elif command -v wget >/dev/null 2>&1; then
    wget -qO "$AGENT_BIN" "$SERVER/downloads/coorpdns-agent-linux"
else
    echo "Erro: curl ou wget necessario para download."
    exit 1
fi
chmod +x "$AGENT_BIN"

echo "[3/4] Registrando servico systemd (coorpdns-agent.service)..."
cat <<EOF > "$SERVICE_FILE"
[Unit]
Description=CoorpDNS Endpoint Protection Agent
After=network.target network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$AGENT_BIN -server $SERVER -token $TOKEN
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable coorpdns-agent.service

echo "[4/4] Iniciando servico e apontando DNS para 127.0.0.1..."
systemctl restart coorpdns-agent.service

# Se o systemd-resolved estiver em uso, configura resolvectl
if command -v resolvectl >/dev/null 2>&1; then
    for IFACE in $(ip -o route get 8.8.8.8 2>/dev/null | awk '{print $5}'); do
        resolvectl dns "$IFACE" 127.0.0.1 2>/dev/null || true
        resolvectl domain "$IFACE" "~." 2>/dev/null || true
    done
fi

echo "================================================="
echo "  OK: CoorpDNS instalado e protegido com sucesso!"
echo "================================================="
`, serverURL, token)

	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(script))
}


