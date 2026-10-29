package web

import (
	"crypto/sha256"
	"embed"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
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
	tokenRegex          = regexp.MustCompile(`^tok_[a-f0-9]{16,64}$`)
	hostRegex           = regexp.MustCompile(`^[a-zA-Z0-9.\-]+(:[0-9]+)?$`)
	domainOnlyRegex     = regexp.MustCompile(`^[a-zA-Z0-9.\-_]{1,253}$`)
	hexColorRegex       = regexp.MustCompile(`^#[0-9a-fA-F]{3,8}$`)
	safeReportTypeRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-]{1,32}$`)
)

func sanitizeCSVCell(val string) string {
	trimmed := strings.TrimLeft(val, " ")
	if trimmed == "" {
		return val
	}
	switch trimmed[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + val
	}
	return val
}

type Server struct {
	cfg                *config.Config
	filter             *filter.Engine
	storage            *storage.DB
	upstream           *dns.UpstreamResolver
	configPath         string
	startTime          time.Time
	staticFS           embed.FS
	loginLimiter       *LoginRateLimiter
	tokenLimiter       *LoginRateLimiter
	onForwardersChange func()
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
		tokenLimiter: NewLoginRateLimiter(),
	}
}

func (s *Server) SetOnForwardersChange(fn func()) {
	s.onForwardersChange = fn
}

func (s *Server) clientIP(r *http.Request) string {
	return getRealClientIP(r, s.cfg.GetTrustedProxies())
}

func (s *Server) isSecure(r *http.Request) bool {
	return isSecureRequest(r, s.cfg.GetTrustedProxies())
}

func (s *Server) isOriginTrusted(origin string, r *http.Request) bool {
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	expectedScheme := "http"
	if s.isSecure(r) {
		expectedScheme = "https"
	}
	if strings.EqualFold(u.Scheme, expectedScheme) && strings.EqualFold(u.Host, r.Host) {
		return true
	}
	_, _, publicURL, _, _, _ := s.cfg.GetSupportInfo()
	if publicURL != "" {
		if pub, err := url.Parse(publicURL); err == nil && pub.Host != "" {
			if strings.EqualFold(u.Scheme, pub.Scheme) && strings.EqualFold(u.Host, pub.Host) {
				return true
			}
		}
	}
	return false
}

func (s *Server) isClientAuthorizedForUser(u *storage.UserWithGroups, clientIP string) bool {
	if u == nil {
		return false
	}
	if u.Role == "admin" {
		return true
	}
	if clientIP == "" || clientIP == "all" {
		return true
	}
	allowedClients, err := s.storage.GetClientsFiltered(u.AllowedGroups)
	if err != nil {
		return false
	}
	for _, ac := range allowedClients {
		if ac.IP == clientIP {
			return true
		}
		if ac.AuthToken != "" {
			if ac.AuthToken == clientIP {
				return true
			}
			prefixLen := 10
			if len(ac.AuthToken) < prefixLen {
				prefixLen = len(ac.AuthToken)
			}
			if "remote-"+ac.AuthToken[:prefixLen] == clientIP {
				return true
			}
		}
	}
	return false
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
	mux.HandleFunc("/api/v1/reports/export-pdf", s.handleExportPDF)

	// Gerenciamento de Clientes, Grupos e Regras
	mux.HandleFunc("/api/v1/clients", s.handleClients)
	mux.HandleFunc("/api/v1/clients/remote", s.handleCreateRemoteClient)
	mux.HandleFunc("/api/v1/clients/revoke", s.handleRevokeRemoteClient)
	mux.HandleFunc("/api/v1/clients/group", s.handleAssignGroup)
	mux.HandleFunc("/api/v1/groups", s.handleGroups)
	mux.HandleFunc("/api/v1/groups/categories", s.handleGroupCategories)
	mux.HandleFunc("/api/v1/groups/applications", s.handleGroupApplications)
	mux.HandleFunc("/api/v1/groups/domains", s.handleGroupDomains)
	mux.HandleFunc("/api/v1/groups/audit", s.handleGroupAudit)
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
		// 1. Cabeçalhos Globais de Segurança (OWASP A05 - Security Hardening, CSP sem unsafe-inline em scripts)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "0")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com https://cdnjs.cloudflare.com; font-src 'self' https://fonts.gstatic.com https://cdnjs.cloudflare.com data:; img-src 'self' data: https:; connect-src 'self' https://wa.me; frame-ancestors 'none'; base-uri 'self'; form-action 'self';")

		if s.isSecure(r) {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		// 2. Política de CORS Segura (correspondência estrita de esquema + host)
		if strings.HasPrefix(r.URL.Path, "/dns-query") {
			// RFC 8484: Endpoints DoH autenticados por token exclusivo
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Authorization, X-CoorpDNS-Token")
		} else {
			origin := r.Header.Get("Origin")
			if origin != "" && s.isOriginTrusted(origin, r) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			}
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		// 3. Proteção Anti-CSRF para operações mutáveis autenticadas por Cookie
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/v1/auth/login" {
			if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete || r.Method == http.MethodPatch {
				_, fromCookie := s.extractSessionToken(r)
				if fromCookie {
					if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
						http.Error(w, "Acesso bloqueado (proteção CSRF: cross-site request)", http.StatusForbidden)
						return
					}
					if origin := r.Header.Get("Origin"); origin != "" && !s.isOriginTrusted(origin, r) {
						http.Error(w, "Acesso bloqueado (proteção CSRF: origem não autorizada)", http.StatusForbidden)
						return
					}
				}
			}
		}

		// 4. Proteção contra DoS por corpo de requisição excessivo (1MB max para API)
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, 1048576) // 1MB max
		}

		// 5. Interceptação de domínios bloqueados sem domínio/IP hardcoded
		host := r.Host
		if idx := strings.Index(host, ":"); idx != -1 {
			host = host[:idx]
		}
		if host != "" && !s.isManagementHost(host) {
			_, _, publicURL, _, _, _ := s.cfg.GetSupportInfo()
			if publicURL != "" {
				cat := s.filter.CategorizeDomain(host)
				redirectURL := fmt.Sprintf("%s/blocked?domain=%s&category=%s", strings.TrimRight(publicURL, "/"), url.QueryEscape(host), url.QueryEscape(cat))
				http.Redirect(w, r, redirectURL, http.StatusFound)
				return
			}
			s.handleBlockedPage(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) isManagementHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return true
	}
	_, serverIP, publicURL, _, _, _ := s.cfg.GetSupportInfo()
	if serverIP != "" {
		cleanSrv := strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(serverIP), "https://"), "http://")
		if idx := strings.Index(cleanSrv, "/"); idx != -1 {
			cleanSrv = cleanSrv[:idx]
		}
		if idx := strings.Index(cleanSrv, ":"); idx != -1 {
			cleanSrv = cleanSrv[:idx]
		}
		if host == cleanSrv {
			return true
		}
	}
	if publicURL != "" {
		if u, err := url.Parse(publicURL); err == nil && strings.EqualFold(u.Hostname(), host) {
			return true
		}
	}
	return false
}

func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	rulesCount, lastUpdate := s.filter.Stats()
	active, pauseRemaining := s.cfg.IsBlockingEffective()

	// Se não autenticado (ex: healthcheck Docker), retorna apenas estado básico sem expor endereço interno
	resp := map[string]interface{}{
		"blocking_active": active,
		"pause_remaining": pauseRemaining,
		"uptime_seconds":  int64(time.Since(s.startTime).Seconds()),
		"status":          "ok",
	}
	if s.getAuthUser(r) != nil {
		resp["total_rules"] = rulesCount
		resp["rules_updated_at"] = lastUpdate.Format(time.RFC3339)
		resp["version"] = "1.0.0"
		resp["dns_address"] = s.cfg.Server.DNSAddress
	}
	jsonResponse(w, http.StatusOK, resp)
}

func (s *Server) handleToggle(w http.ResponseWriter, r *http.Request) {
	admin := s.requireAdmin(w, r)
	if admin == nil {
		return
	}

	if r.Method != http.MethodPost {
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
	if req.DurationSeconds < 0 || req.DurationSeconds > 86400*7 {
		http.Error(w, "Duração de pausa inválida", http.StatusBadRequest)
		return
	}

	s.cfg.SetBlockingState(req.Enabled, req.DurationSeconds)
	_ = s.cfg.Save(s.configPath)
	active, _ := s.cfg.IsBlockingEffective()
	jsonResponse(w, http.StatusOK, map[string]interface{}{"success": true, "blocking_active": active})
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

	if client != "" && !s.isClientAuthorizedForUser(u, client) {
		http.Error(w, "Acesso negado a este dispositivo", http.StatusForbidden)
		return
	}

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

	clientIP := strings.TrimSpace(r.URL.Query().Get("client_ip"))
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	fromDate := strings.TrimSpace(r.URL.Query().Get("from"))
	toDate := strings.TrimSpace(r.URL.Query().Get("to"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	if !s.isClientAuthorizedForUser(u, clientIP) {
		http.Error(w, "Acesso negado a este dispositivo", http.StatusForbidden)
		return
	}

	var allowedGroups []int64
	if u.Role != "admin" {
		allowedGroups = u.AllowedGroups
	}

	items, err := s.storage.GetClientDomainActivityFiltered(clientIP, days, fromDate, toDate, limit, allowedGroups)
	if err != nil {
		http.Error(w, "Erro ao consultar histórico do dispositivo", http.StatusInternalServerError)
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

	reportType := strings.TrimSpace(r.URL.Query().Get("type"))
	if !safeReportTypeRegex.MatchString(reportType) {
		reportType = "client"
	}
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 || days > 180 {
		days = 180
	}

	var allowedGroups []int64
	if u.Role != "admin" {
		allowedGroups = u.AllowedGroups
	}

	// Valida permissões de grupo/dispositivo ANTES de enviar cabeçalhos de download CSV
	var groupID int64
	var clientIP string
	if reportType == "group" {
		groupID, _ = strconv.ParseInt(r.URL.Query().Get("group_id"), 10, 64)
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
	} else if reportType != "domains" && reportType != "clients" {
		clientIP = strings.TrimSpace(r.URL.Query().Get("client_ip"))
		if !s.isClientAuthorizedForUser(u, clientIP) {
			http.Error(w, "Acesso negado a este dispositivo", http.StatusForbidden)
			return
		}
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"relatorio_%s_%ddias.csv\"", reportType, days))

	writer := csv.NewWriter(w)
	defer writer.Flush()

	if reportType == "domains" {
		writer.Write([]string{"Dominio", "Total de Consultas", "Consultas Bloqueadas", "Taxa de Bloqueio (%)"})
		items, _ := s.storage.GetTopDomainsFiltered(days, 500, "all", allowedGroups)
		for _, it := range items {
			writer.Write([]string{
				sanitizeCSVCell(it.Domain),
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
				sanitizeCSVCell(it.ClientIP),
				sanitizeCSVCell(it.ClientName),
				strconv.FormatInt(it.TotalQueries, 10),
				strconv.FormatInt(it.BlockedQueries, 10),
				fmt.Sprintf("%.1f%%", it.BlockRate),
			})
		}
	} else if reportType == "group" {
		rep, _ := s.storage.GetGroupReport(groupID, days, 200)
		writer.Write([]string{"Grupo", "Dispositivo", "IP", "Dominio", "Total de Consultas", "Bloqueadas"})
		if rep != nil {
			for _, dev := range rep.Devices {
				for _, dom := range dev.Domains {
					writer.Write([]string{
						sanitizeCSVCell(rep.GroupName),
						sanitizeCSVCell(dev.ClientName),
						sanitizeCSVCell(dev.ClientIP),
						sanitizeCSVCell(dom.Domain),
						strconv.FormatInt(dom.TotalQueries, 10),
						strconv.FormatInt(dom.BlockedQueries, 10),
					})
				}
			}
		}
	} else {
		writer.Write([]string{"Dominio", "Total de Consultas", "Bloqueadas"})
		items, _ := s.storage.GetClientDomainActivityFiltered(clientIP, days, "", "", 500, allowedGroups)
		for _, it := range items {
			writer.Write([]string{
				sanitizeCSVCell(it.Domain),
				strconv.FormatInt(it.TotalQueries, 10),
				strconv.FormatInt(it.BlockedQueries, 10),
			})
		}
	}
}

func (s *Server) handleExportPDF(w http.ResponseWriter, r *http.Request) {
	u := s.requireAuth(w, r)
	if u == nil {
		return
	}

	groupID, _ := strconv.ParseInt(r.URL.Query().Get("group_id"), 10, 64)
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 || days > 180 {
		days = 7
	}
	if groupID <= 0 {
		if u.Role != "admin" && len(u.AllowedGroups) > 0 {
			groupID = u.AllowedGroups[0]
		} else {
			groupID = 1
		}
	}

	if u.Role != "admin" && !containsInt64(u.AllowedGroups, groupID) {
		http.Error(w, "Acesso negado para exportar relatório deste grupo", http.StatusForbidden)
		return
	}

	report, err := s.storage.GetGroupDetailedReportFiltered(groupID, days, 50, u.AllowedGroups)
	if err != nil {
		http.Error(w, "Erro ao consolidar dados do relatório", http.StatusInternalServerError)
		return
	}

	pdfBytes, err := GenerateGroupReportPDF(report, days)
	if err != nil {
		http.Error(w, "Erro ao gerar arquivo PDF: "+err.Error(), http.StatusInternalServerError)
		return
	}

	safeName := safeFilenameRegex.ReplaceAllString(report.GroupName, "_")
	if safeName == "" {
		safeName = "grupo"
	}
	filename := fmt.Sprintf("relatorio-coorpdns-%s-%ddias.pdf", safeName, days)

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(pdfBytes)))
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(http.StatusOK)
	w.Write(pdfBytes)
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
		if name == "" || len(name) > 80 {
			http.Error(w, "Nome do grupo é obrigatório (máx 80 caracteres)", http.StatusBadRequest)
			return
		}
		color := strings.TrimSpace(req.Color)
		if !hexColorRegex.MatchString(color) {
			color = "#10b981"
		}
		g, err := s.storage.CreateGroup(name, strings.TrimSpace(req.Description), color)
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
	s.storage.AddGroupAuditLog(req.GroupID, u.Username, u.FullName, "device_assigned", fmt.Sprintf("Dispositivo %s associado a este grupo", req.ClientIP))
	jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleClients(w http.ResponseWriter, r *http.Request) {
	u := s.requireAuth(w, r)
	if u == nil {
		return
	}

	if r.Method == http.MethodPost {
		// DTO estrito para impedir Mass Assignment de auth_token, is_remote ou status
		var req struct {
			IP           string `json:"ip"`
			Name         string `json:"name"`
			MAC          string `json:"mac"`
			Comment      string `json:"comment"`
			BlockEnabled *bool  `json:"block_enabled"`
			GroupID      int64  `json:"group_id"`
			OSType       string `json:"os_type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid body", http.StatusBadRequest)
			return
		}

		req.IP = strings.TrimSpace(req.IP)
		req.Name = strings.TrimSpace(req.Name)
		req.MAC = strings.TrimSpace(req.MAC)
		req.Comment = strings.TrimSpace(req.Comment)

		if req.IP == "" || req.Name == "" || len(req.Name) > 120 {
			http.Error(w, "IP/identificador e nome válido são obrigatórios", http.StatusBadRequest)
			return
		}

		existing, err := s.storage.GetClientByIP(req.IP)
		isExistingRemote := err == nil && existing != nil && existing.IsRemote
		if !isExistingRemote && net.ParseIP(req.IP) == nil {
			http.Error(w, "Endereço IP inválido", http.StatusBadRequest)
			return
		}

		targetGroupID := req.GroupID
		if targetGroupID <= 0 {
			if existing != nil && existing.GroupID > 0 {
				targetGroupID = existing.GroupID
			} else if u.Role != "admin" && len(u.AllowedGroups) > 0 {
				targetGroupID = u.AllowedGroups[0]
			} else {
				targetGroupID = 1
			}
		}

		if u.Role != "admin" {
			if !containsInt64(u.AllowedGroups, targetGroupID) {
				http.Error(w, "Acesso negado para o grupo informado", http.StatusForbidden)
				return
			}
			if existing != nil && existing.GroupID > 0 && !containsInt64(u.AllowedGroups, existing.GroupID) {
				http.Error(w, "Acesso negado: dispositivo pertence a outro departamento", http.StatusForbidden)
				return
			}
		}

		blockEnabled := true
		if req.BlockEnabled != nil {
			blockEnabled = *req.BlockEnabled
		} else if existing != nil {
			blockEnabled = existing.BlockEnabled
		}

		c := storage.Client{
			IP:           req.IP,
			Name:         req.Name,
			MAC:          req.MAC,
			Comment:      req.Comment,
			BlockEnabled: blockEnabled,
			GroupID:      targetGroupID,
			OSType:       req.OSType,
			Status:       "active",
		}
		if existing != nil {
			c.AuthToken = existing.AuthToken
			c.IsRemote = existing.IsRemote
			c.Status = existing.Status
			if c.OSType == "" {
				c.OSType = existing.OSType
			}
		}

		if err := s.storage.UpsertClient(&c); err != nil {
			http.Error(w, "Erro ao salvar dispositivo", http.StatusInternalServerError)
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

	s.storage.AddGroupAuditLog(req.GroupID, u.Username, u.FullName, "device_created", fmt.Sprintf("Dispositivo remoto criado: %s (%s)", client.Name, client.IP))
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

	s.storage.AddGroupAuditLog(client.GroupID, u.Username, u.FullName, "device_revoked", fmt.Sprintf("Dispositivo remoto revogado: %s", client.Name))
	jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleRules(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
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
		if rule == "" || len(rule) > 255 || strings.ContainsAny(rule, " \t\r\n\"'<>") {
			http.Error(w, "Regra inválida", http.StatusBadRequest)
			return
		}
		switch req.Action {
		case "add_block", "remove_block", "add_allow", "remove_allow":
		default:
			http.Error(w, "Ação de regra inválida", http.StatusBadRequest)
			return
		}

		customRules, allowRules := s.cfg.ModifyRule(req.Action, rule)
		s.filter.LoadRules(nil, customRules, allowRules)
		_ = s.cfg.Save(s.configPath)
		jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
		return
	}

	u := s.requireAuth(w, r)
	if u == nil {
		return
	}

	customRules, allowRules := s.cfg.GetRules()
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"custom_rules": customRules,
		"allow_rules":  allowRules,
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
		catBlockedCount := 0
		for _, act := range req.Rules {
			if strings.ToLower(strings.TrimSpace(act)) == "block" {
				catBlockedCount++
			}
		}
		s.storage.AddGroupAuditLog(req.GroupID, u.Username, u.FullName, "categories", fmt.Sprintf("Políticas de categoria salvas (%d bloqueadas)", catBlockedCount))
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
		appBlockedCount := 0
		for _, act := range req.Rules {
			if strings.ToLower(strings.TrimSpace(act)) == "block" {
				appBlockedCount++
			}
		}
		s.storage.AddGroupAuditLog(req.GroupID, u.Username, u.FullName, "applications", fmt.Sprintf("Políticas de aplicações salvas (%d bloqueadas)", appBlockedCount))
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
		statusTxt := "desativado"
		if req.Schedule.Enabled {
			statusTxt = fmt.Sprintf("ativo (modo: %s, %d blocos)", req.Schedule.Mode, len(req.Schedule.Slots))
		}
		s.storage.AddGroupAuditLog(req.GroupID, u.Username, u.FullName, "schedule", fmt.Sprintf("Horário de funcionamento salvo: %s", statusTxt))
		jsonResponse(w, http.StatusOK, map[string]interface{}{"success": true, "group_id": req.GroupID, "schedule": req.Schedule})
		return
	}
}

func (s *Server) handleGroupDomains(w http.ResponseWriter, r *http.Request) {
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
		rules, err := s.storage.GetGroupDomainRules(groupID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"group_id": groupID,
			"domains":  rules,
		})
		return
	}

	if r.Method == "POST" {
		var req struct {
			GroupID int64  `json:"group_id"`
			Domain  string `json:"domain"`
			Action  string `json:"action"` // "block" ou "allow"
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid body", http.StatusBadRequest)
			return
		}
		if req.GroupID <= 0 {
			req.GroupID = 1
		}
		if u.Role != "admin" && !containsInt64(u.AllowedGroups, req.GroupID) {
			http.Error(w, "Acesso negado para gerenciar domínios deste grupo", http.StatusForbidden)
			return
		}

		cleanDomain, err := filter.NormalizeCustomDomainInput(req.Domain)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		action := strings.ToLower(strings.TrimSpace(req.Action))
		if action != "allow" {
			action = "block"
		}

		if err := s.storage.UpsertGroupDomainRule(req.GroupID, cleanDomain, action); err != nil {
			http.Error(w, "Erro ao salvar regra de domínio", http.StatusInternalServerError)
			return
		}

		actionDesc := "bloqueado"
		if action == "allow" {
			actionDesc = "liberado (exceção)"
		}
		s.storage.AddGroupAuditLog(req.GroupID, u.Username, u.FullName, "domain_rule", fmt.Sprintf("Domínio '%s' cadastrado como %s", cleanDomain, actionDesc))

		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"domain":  cleanDomain,
			"action":  action,
		})
		return
	}

	if r.Method == "DELETE" {
		groupID, _ := strconv.ParseInt(r.URL.Query().Get("group_id"), 10, 64)
		ruleID, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if groupID <= 0 || ruleID <= 0 {
			http.Error(w, "Parâmetros group_id e id obrigatórios", http.StatusBadRequest)
			return
		}
		if u.Role != "admin" && !containsInt64(u.AllowedGroups, groupID) {
			http.Error(w, "Acesso negado para gerenciar domínios deste grupo", http.StatusForbidden)
			return
		}

		if err := s.storage.DeleteGroupDomainRule(groupID, ruleID); err != nil {
			http.Error(w, "Erro ao remover regra de domínio", http.StatusInternalServerError)
			return
		}
		s.storage.AddGroupAuditLog(groupID, u.Username, u.FullName, "domain_deleted", fmt.Sprintf("Regra de domínio ID %d removida", ruleID))
		jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (s *Server) handleGroupAudit(w http.ResponseWriter, r *http.Request) {
	u := s.requireAuth(w, r)
	if u == nil {
		return
	}

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

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	logs, err := s.storage.GetGroupAuditLogs(groupID, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"group_id": groupID,
		"logs":     logs,
	})
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

	var allowedGroups []int64
	if u.Role != "admin" {
		allowedGroups = u.AllowedGroups
	}
	clients, _ := s.storage.GetClientsFiltered(allowedGroups)
	total := 0
	for _, cl := range clients {
		if cl.Status == "active" {
			total++
		}
	}
	online := total

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

	if r.Method == http.MethodPost {
		var req struct {
			Domain     string `json:"domain"`
			UpstreamIP string `json:"upstream_ip"`
			Comment    string `json:"comment"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid body", http.StatusBadRequest)
			return
		}
		domain := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(req.Domain)), ".")
		if !domainOnlyRegex.MatchString(domain) {
			http.Error(w, "Domínio inválido", http.StatusBadRequest)
			return
		}
		upstreamAddr := strings.TrimSpace(req.UpstreamIP)
		upstreamHost := upstreamAddr
		if h, p, err := net.SplitHostPort(upstreamAddr); err == nil {
			portNum, pErr := strconv.Atoi(p)
			if pErr != nil || portNum < 1 || portNum > 65535 {
				http.Error(w, "Porta de upstream inválida", http.StatusBadRequest)
				return
			}
			upstreamHost = h
		}
		if net.ParseIP(upstreamHost) == nil {
			http.Error(w, "Endereço IP do servidor DNS upstream inválido", http.StatusBadRequest)
			return
		}

		f, err := s.storage.CreateConditionalForwarder(domain, upstreamAddr, strings.TrimSpace(req.Comment))
		if err != nil {
			http.Error(w, "Erro ao criar encaminhador condicional", http.StatusBadRequest)
			return
		}
		s.reloadWebAndDNSForwarders()
		jsonResponse(w, http.StatusCreated, f)
		return
	}

	if r.Method == http.MethodDelete {
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) >= 5 {
			id, _ := strconv.ParseInt(parts[4], 10, 64)
			if id > 0 {
				_ = s.storage.DeleteConditionalForwarder(id)
				s.reloadWebAndDNSForwarders()
				jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
				return
			}
		}
		http.Error(w, "ID not provided", http.StatusBadRequest)
		return
	}
}

func (s *Server) reloadWebAndDNSForwarders() {
	if list, err := s.storage.GetConditionalForwarders(); err == nil {
		m := make(map[string]string)
		for _, f := range list {
			if f.Enabled {
				m[f.Domain] = f.UpstreamIP
			}
		}
		s.upstream.SetConditionalForwarders(m)
	}
	if s.onForwardersChange != nil {
		s.onForwardersChange()
	}
}

func (s *Server) handleSupportConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		blockMode, serverIP, publicURL, email, phone, org := s.cfg.GetSupportInfo()
		// Acesso público para preenchimento de contatos de suporte na página de bloqueio (sem expor IP interno desnecessariamente se não autenticado)
		resp := map[string]interface{}{
			"block_mode":    blockMode,
			"support_email": email,
			"support_phone": phone,
			"org_name":      org,
		}
		if s.getAuthUser(r) != nil {
			resp["server_ip"] = serverIP
			resp["public_url"] = publicURL
		}
		jsonResponse(w, http.StatusOK, resp)
		return
	}

	admin := s.requireAdmin(w, r)
	if admin == nil {
		return
	}

	if r.Method == http.MethodPut || r.Method == http.MethodPost {
		var req struct {
			BlockMode    string `json:"block_mode"`
			ServerIP     string `json:"server_ip"`
			PublicURL    string `json:"public_url"`
			SupportEmail string `json:"support_email"`
			SupportPhone string `json:"support_phone"`
			OrgName      string `json:"org_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid body", http.StatusBadRequest)
			return
		}
		s.cfg.UpdateSupportInfo(
			strings.TrimSpace(req.BlockMode),
			strings.TrimSpace(req.ServerIP),
			strings.TrimSpace(req.PublicURL),
			strings.TrimSpace(req.SupportEmail),
			strings.TrimSpace(req.SupportPhone),
			strings.TrimSpace(req.OrgName),
		)
		_ = s.cfg.Save(s.configPath)
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

func findDownloadBinaryPath(filename string) string {
	searchPaths := []string{
		filepath.Join("./downloads", filename),
		filepath.Join("/app/downloads", filename),
		filepath.Join("./data/downloads", filename),
	}
	for _, p := range searchPaths {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

func computeFileSHA256(filePath string) string {
	if filePath == "" {
		return ""
	}
	f, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

func resolveInstallerServerURL(r *http.Request, isSecure bool) string {
	host := r.Host
	hostOnly := host
	if idx := strings.Index(hostOnly, ":"); idx != -1 {
		hostOnly = hostOnly[:idx]
	}
	if !isSecure && (hostOnly == "localhost" || hostOnly == "127.0.0.1") {
		return "http://" + host
	}
	// Impede downgrade HTTP em redes externas/remotas
	return "https://" + host
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
	}
	if !allowedFiles[filename] {
		http.Error(w, "Acesso a arquivo restrito ou não permitido", http.StatusForbidden)
		return
	}

	filePath := findDownloadBinaryPath(filename)
	if filePath == "" {
		http.Error(w, fmt.Sprintf("Arquivo %s ainda não disponível no servidor", filename), http.StatusNotFound)
		return
	}

	if hashHex := computeFileSHA256(filePath); hashHex != "" {
		w.Header().Set("X-Content-SHA256", hashHex)
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

func (s *Server) validateInstallerToken(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	clientIP := s.clientIP(r)
	rateKey := "install:" + clientIP
	if s.tokenLimiter != nil && !s.tokenLimiter.AllowWithLimit(rateKey, 20, 5*time.Minute) {
		http.Error(w, "Muitas requisições inválidas ao instalador. Aguarde 5 minutos.", http.StatusTooManyRequests)
		return "", "", false
	}

	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if !tokenRegex.MatchString(token) {
		if s.tokenLimiter != nil {
			s.tokenLimiter.RecordFailedAttempt(rateKey)
		}
		http.Error(w, "Token de endpoint corporativo inválido. Formato esperado: tok_[a-f0-9]{16,64}", http.StatusBadRequest)
		return "", "", false
	}

	client, err := s.storage.GetClientByToken(token)
	if err != nil || client == nil || client.Status != "active" {
		if s.tokenLimiter != nil {
			s.tokenLimiter.RecordFailedAttempt(rateKey)
		}
		http.Error(w, "Token de dispositivo não encontrado ou revogado.", http.StatusForbidden)
		return "", "", false
	}

	host := r.Host
	if !hostRegex.MatchString(host) {
		http.Error(w, "Host header inválido", http.StatusBadRequest)
		return "", "", false
	}

	serverURL := resolveInstallerServerURL(r, s.isSecure(r))
	return token, serverURL, true
}

func (s *Server) handleInstallScript(w http.ResponseWriter, r *http.Request) {
	token, serverURL, ok := s.validateInstallerToken(w, r)
	if !ok {
		return
	}

	expectedSHA256 := computeFileSHA256(findDownloadBinaryPath("coorpdns-agent.exe"))

	script := fmt.Sprintf(`# CoorpDNS Windows Enterprise Silent One-Liner Installer
$ErrorActionPreference = "Stop"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$Server = "%s"
$Token = "%s"
$ExpectedSHA256 = "%s"
$InstallDir = "$env:ProgramFiles\CoorpDNS"

# 0. Verificacao de Privilegios de Administrador
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "=================================================" -ForegroundColor Red
    Write-Host "  ERRO: PRIVILEGIOS DE ADMINISTRADOR NECESSARIOS" -ForegroundColor Red
    Write-Host "=================================================" -ForegroundColor Red
    Exit 1
}

if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

Write-Host "[1/4] Finalizando instancias anteriores..." -ForegroundColor Yellow
Stop-Process -Name "coorpdns-agent" -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 1

Write-Host "[2/4] Baixando e verificando integridade SHA-256 do coorpdns-agent.exe..." -ForegroundColor Yellow
$AgentPath = "$InstallDir\coorpdns-agent.exe"
$Resp = Invoke-WebRequest -Uri "$Server/downloads/coorpdns-agent.exe" -OutFile $AgentPath -PassThru -UseBasicParsing
$ActualSHA256 = (Get-FileHash -Path $AgentPath -Algorithm SHA256).Hash.ToLower()
$PinnedSHA256 = $ExpectedSHA256
if ([string]::IsNullOrEmpty($PinnedSHA256) -and $Resp.Headers["X-Content-SHA256"]) {
    $PinnedSHA256 = ([string]$Resp.Headers["X-Content-SHA256"]).ToLower()
}
if ([string]::IsNullOrEmpty($PinnedSHA256) -or $ActualSHA256 -ne $PinnedSHA256) {
    Remove-Item -Path $AgentPath -Force -ErrorAction SilentlyContinue
    throw "ERRO CRITICO DE SEGURANCA: Hash SHA-256 do binario nao confere!"
}

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

Write-Host "OK: CoorpDNS instalado e verificado com sucesso!" -ForegroundColor Green
`, serverURL, token, expectedSHA256)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(script))
}

func (s *Server) handleInstallLinuxScript(w http.ResponseWriter, r *http.Request) {
	token, serverURL, ok := s.validateInstallerToken(w, r)
	if !ok {
		return
	}

	expectedSHA256 := computeFileSHA256(findDownloadBinaryPath("coorpdns-agent-linux"))

	script := fmt.Sprintf(`#!/usr/bin/env bash
# CoorpDNS Linux Enterprise Silent One-Liner Installer
set -euo pipefail

SERVER="%s"
TOKEN="%s"
EXPECTED_SHA256="%s"
INSTALL_DIR="/usr/local/bin"
AGENT_BIN="$INSTALL_DIR/coorpdns-agent"
SERVICE_FILE="/etc/systemd/system/coorpdns-agent.service"

if [ "$(id -u)" -ne 0 ]; then
    echo "ERRO: PRIVILEGIOS DE ROOT / SUDO NECESSARIOS"
    exit 1
fi

systemctl stop coorpdns-agent 2>/dev/null || true
pkill -f "coorpdns-agent" 2>/dev/null || true

mkdir -p "$INSTALL_DIR"
TMP_BIN="$(mktemp)"
trap 'rm -f "$TMP_BIN"' EXIT

if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$SERVER/downloads/coorpdns-agent-linux" -o "$TMP_BIN"
elif command -v wget >/dev/null 2>&1; then
    wget -qO "$TMP_BIN" "$SERVER/downloads/coorpdns-agent-linux"
else
    echo "Erro: curl ou wget necessario para download."
    exit 1
fi

if [ -n "$EXPECTED_SHA256" ]; then
    echo "${EXPECTED_SHA256}  ${TMP_BIN}" | sha256sum -c -
fi

install -m 0755 "$TMP_BIN" "$AGENT_BIN"

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
chmod 0600 "$SERVICE_FILE"

systemctl daemon-reload
systemctl enable coorpdns-agent.service
systemctl restart coorpdns-agent.service

if command -v resolvectl >/dev/null 2>&1; then
    for IFACE in $(ip -o route get 8.8.8.8 2>/dev/null | awk '{print $5}'); do
        resolvectl dns "$IFACE" 127.0.0.1 2>/dev/null || true
        resolvectl domain "$IFACE" "~." 2>/dev/null || true
    done
fi

echo "OK: CoorpDNS instalado e verificado com sucesso!"
`, serverURL, token, expectedSHA256)

	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(script))
}


