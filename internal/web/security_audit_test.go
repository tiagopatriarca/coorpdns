package web

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coorpdns/coorpdns/internal/config"
	"github.com/coorpdns/coorpdns/internal/filter"
	"github.com/coorpdns/coorpdns/internal/storage"
)

func setupHardenedTestServer(t *testing.T) (*Server, *storage.DB, http.Handler, string) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "audit_test.db")
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	cfg := config.DefaultConfig()
	cfg.Storage.DBPath = dbPath
	cfg.Server.TrustedProxies = []string{"127.0.0.1/32"}
	if err := cfg.Save(cfgPath); err != nil {
		t.Fatalf("falha ao salvar config de teste: %v", err)
	}

	store, err := storage.NewDB(dbPath, 30, 7)
	if err != nil {
		t.Fatalf("falha ao iniciar banco de teste: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	eng := filter.NewEngine()
	var emptyFS embed.FS
	srv := NewServer(cfg, eng, store, cfgPath, emptyFS)
	return srv, store, srv.Routes(), tmpDir
}

func TestSecurity_RBAC_AntiIDOR_ReportsAndCSV(t *testing.T) {
	_, store, handler, _ := setupHardenedTestServer(t)

	// Grupo 2: autorizado ao operador; Grupo 3: restrito (Financeiro/Diretoria)
	opUser, err := store.CreateUser("operador_g2", "SenhaForte#2026", "Operador G2", "operator", []int64{2})
	if err != nil {
		t.Fatalf("erro ao criar operador: %v", err)
	}
	opSession, err := store.CreateSession(opUser.ID, 30*time.Minute)
	if err != nil {
		t.Fatalf("erro ao criar sessão do operador: %v", err)
	}

	// Cliente local e remoto no Grupo 3 (NÃO autorizado ao operador_g2)
	if err := store.UpsertClient(&storage.Client{
		IP:           "10.30.0.55",
		Name:         "Estacao-Diretoria",
		BlockEnabled: true,
		Status:       "active",
		GroupID:      3,
	}); err != nil {
		t.Fatalf("erro ao cadastrar cliente local G3: %v", err)
	}
	remoteG3, err := store.CreateRemoteClient("Notebook-CEO", "Remoto G3", "windows", 3)
	if err != nil {
		t.Fatalf("erro ao cadastrar cliente remoto G3: %v", err)
	}

	// Registra atividade confidencial no Grupo 3 e atividade permitida no Grupo 2
	if err := store.UpsertClient(&storage.Client{
		IP:           "10.20.0.10",
		Name:         "TV-Sala-G2",
		BlockEnabled: true,
		Status:       "active",
		GroupID:      2,
	}); err != nil {
		t.Fatalf("erro ao cadastrar cliente G2: %v", err)
	}

	store.LogQuery(&storage.QueryLogRecord{
		Timestamp: time.Now(),
		ClientIP:  "10.30.0.55",
		Domain:    "confidencial-diretoria.interno",
		QType:     "A",
		Status:    "allowed",
	})
	store.LogQuery(&storage.QueryLogRecord{
		Timestamp: time.Now(),
		ClientIP:  remoteG3.IP,
		Domain:    "remoto-secreto-ceo.interno",
		QType:     "A",
		Status:    "allowed",
	})
	store.LogQuery(&storage.QueryLogRecord{
		Timestamp: time.Now(),
		ClientIP:  "10.20.0.10",
		Domain:    "permitido-grupo2.com",
		QType:     "A",
		Status:    "allowed",
	})

	// Aguarda flush assíncrono do worker SQLite
	time.Sleep(2300 * time.Millisecond)

	// 1. Tentativa de IDOR direto no IP local de outro grupo -> deve retornar 403
	req1 := httptest.NewRequest("GET", "/api/v1/reports/client-activity?client_ip=10.30.0.55&days=7", nil)
	req1.Host = "localhost:8080"
	req1.AddCookie(&http.Cookie{Name: "coorpdns_session", Value: opSession})
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusForbidden {
		t.Errorf("IDOR local IP: esperado 403, obteve %d (%s)", rr1.Code, rr1.Body.String())
	}

	// 2. Tentativa de bypass usando prefixo remote-* de outro grupo -> deve retornar 403
	req2 := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/reports/client-activity?client_ip=%s&days=7", remoteG3.IP), nil)
	req2.Host = "localhost:8080"
	req2.AddCookie(&http.Cookie{Name: "coorpdns_session", Value: opSession})
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusForbidden {
		t.Errorf("IDOR remote-* IP: esperado 403, obteve %d (%s)", rr2.Code, rr2.Body.String())
	}

	// 3. Consulta agregada client_ip=all -> deve exibir apenas dados do Grupo 2 e ocultar Grupo 3
	req3 := httptest.NewRequest("GET", "/api/v1/reports/client-activity?client_ip=all&days=7", nil)
	req3.Host = "localhost:8080"
	req3.AddCookie(&http.Cookie{Name: "coorpdns_session", Value: opSession})
	rr3 := httptest.NewRecorder()
	handler.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Fatalf("client_ip=all: esperado 200, obteve %d", rr3.Code)
	}
	bodyAll := rr3.Body.String()
	if strings.Contains(bodyAll, "confidencial-diretoria.interno") || strings.Contains(bodyAll, "remoto-secreto-ceo.interno") {
		t.Errorf("vazamento de dados entre grupos em client_ip=all: %s", bodyAll)
	}
	if !strings.Contains(bodyAll, "permitido-grupo2.com") {
		t.Errorf("esperava encontrar atividade legítima do Grupo 2 em client_ip=all, obteve: %s", bodyAll)
	}

	// 4. Exportação CSV matrix para IP de outro grupo -> deve retornar 403
	req4 := httptest.NewRequest("GET", "/api/v1/reports/export-csv?type=matrix&client_ip=10.30.0.55&days=7", nil)
	req4.Host = "localhost:8080"
	req4.AddCookie(&http.Cookie{Name: "coorpdns_session", Value: opSession})
	rr4 := httptest.NewRecorder()
	handler.ServeHTTP(rr4, req4)
	if rr4.Code != http.StatusForbidden {
		t.Errorf("IDOR CSV matrix: esperado 403, obteve %d", rr4.Code)
	}
}

func TestSecurity_ClientsMassAssignmentAndGroupEnforcement(t *testing.T) {
	_, store, handler, _ := setupHardenedTestServer(t)

	opUser, err := store.CreateUser("op_restrito", "SenhaSegura!99", "Operador Restrito", "operator", []int64{2})
	if err != nil {
		t.Fatalf("erro ao criar operador: %v", err)
	}
	opSession, _ := store.CreateSession(opUser.ID, 30*time.Minute)

	// 1. Operador tentando cadastrar cliente no Grupo 4 (não autorizado) -> 403
	payloadUnauth := `{"ip":"192.168.50.10","name":"PC","group_id":4,"auth_token":"tok_deadbeefcafebabe1234","is_remote":true}`
	req1 := httptest.NewRequest("POST", "/api/v1/clients", strings.NewReader(payloadUnauth))
	req1.Host = "localhost:8080"
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Origin", "http://localhost:8080")
	req1.AddCookie(&http.Cookie{Name: "coorpdns_session", Value: opSession})
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusForbidden {
		t.Errorf("esperado 403 ao tentar vincular grupo não autorizado, obteve %d", rr1.Code)
	}

	// 2. Operador cadastrando no Grupo 2 (autorizado), mas tentando Mass Assignment de auth_token / is_remote
	payloadMass := `{"ip":"192.168.50.20","name":"PC Valido","group_id":2,"auth_token":"tok_deadbeefcafebabe1234","is_remote":true}`
	req2 := httptest.NewRequest("POST", "/api/v1/clients", strings.NewReader(payloadMass))
	req2.Host = "localhost:8080"
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Origin", "http://localhost:8080")
	req2.AddCookie(&http.Cookie{Name: "coorpdns_session", Value: opSession})
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("esperado 200 no grupo autorizado, obteve %d: %s", rr2.Code, rr2.Body.String())
	}

	saved, err := store.GetClientByIP("192.168.50.20")
	if err != nil {
		t.Fatalf("cliente não encontrado: %v", err)
	}
	if saved.AuthToken != "" || saved.IsRemote {
		t.Errorf("Mass Assignment não foi bloqueado! AuthToken=%q, IsRemote=%v", saved.AuthToken, saved.IsRemote)
	}
}

func TestSecurity_RateLimitIgnoresSpoofedXFF(t *testing.T) {
	_, _, handler, _ := setupHardenedTestServer(t)

	for i := 1; i <= 6; i++ {
		body := bytes.NewBufferString(`{"username":"admin","password":"wrong_password"}`)
		req := httptest.NewRequest("POST", "/api/v1/auth/login", body)
		req.Host = "localhost:8080"
		// Origem real NÃO está em TrustedProxies (127.0.0.1/32), mas envia XFF falso diferente a cada tentativa
		req.RemoteAddr = "198.51.100.77:54321"
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("203.0.113.%d", i))
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if i <= 5 && rr.Code != http.StatusUnauthorized {
			t.Fatalf("tentativa %d: esperado 401, obteve %d", i, rr.Code)
		}
		if i == 6 && rr.Code != http.StatusTooManyRequests {
			t.Errorf("6ª tentativa com XFF falsificado deveria retornar 429 Too Many Requests, obteve %d", rr.Code)
		}
	}
}

func TestSecurity_SessionQueryRejected_CSRF_CSP_And_SessionRevocation(t *testing.T) {
	_, store, handler, _ := setupHardenedTestServer(t)

	u, err := store.CreateUser("auditor", "AuditPass#2026", "Auditor Sec", "admin", nil)
	if err != nil {
		t.Fatalf("erro ao criar usuário: %v", err)
	}
	sess1, _ := store.CreateSession(u.ID, 30*time.Minute)
	sess2, _ := store.CreateSession(u.ID, 30*time.Minute)

	// 1. ?session= na URL deve ser rejeitado (401)
	reqQuery := httptest.NewRequest("GET", "/api/v1/auth/me?session="+sess1, nil)
	reqQuery.Host = "localhost:8080"
	rrQuery := httptest.NewRecorder()
	handler.ServeHTTP(rrQuery, reqQuery)
	if rrQuery.Code != http.StatusUnauthorized {
		t.Errorf("esperava 401 para ?session= na URL, obteve %d", rrQuery.Code)
	}

	// 2. Verifica cabeçalho CSP sem 'unsafe-inline' em script-src
	csp := rrQuery.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self'") || strings.Contains(csp, "script-src 'self' 'unsafe-inline'") {
		t.Errorf("CSP inseguro em script-src: %q", csp)
	}

	// 3. Verifica proteção CSRF em requisição mutável autenticada por cookie com Origin externo
	reqCSRF := httptest.NewRequest("POST", "/api/v1/control/toggle", strings.NewReader(`{"enabled":false}`))
	reqCSRF.Host = "localhost:8080"
	reqCSRF.Header.Set("Origin", "https://attacker.invalid")
	reqCSRF.Header.Set("Content-Type", "application/json")
	reqCSRF.AddCookie(&http.Cookie{Name: "coorpdns_session", Value: sess1})
	rrCSRF := httptest.NewRecorder()
	handler.ServeHTTP(rrCSRF, reqCSRF)
	if rrCSRF.Code != http.StatusForbidden {
		t.Errorf("CSRF cross-origin deveria retornar 403, obteve %d", rrCSRF.Code)
	}

	// 4. CORS não deve refletir origem maliciosa parecida com localhost
	if gotOrigin := rrCSRF.Header().Get("Access-Control-Allow-Origin"); gotOrigin != "" {
		t.Errorf("CORS refletiu origem não confiável: %q", gotOrigin)
	}

	// 5. Troca de senha com menos de 8 caracteres deve falhar (400)
	reqShort := httptest.NewRequest("POST", "/api/v1/auth/password", strings.NewReader(`{"old_password":"AuditPass#2026","new_password":"123456"}`))
	reqShort.Host = "localhost:8080"
	reqShort.Header.Set("Origin", "http://localhost:8080")
	reqShort.Header.Set("Content-Type", "application/json")
	reqShort.AddCookie(&http.Cookie{Name: "coorpdns_session", Value: sess1})
	rrShort := httptest.NewRecorder()
	handler.ServeHTTP(rrShort, reqShort)
	if rrShort.Code != http.StatusBadRequest {
		t.Errorf("senha curta deveria retornar 400, obteve %d", rrShort.Code)
	}

	// 6. Troca de senha válida deve revogar sessões paralelas (sess2)
	reqPwd := httptest.NewRequest("POST", "/api/v1/auth/password", strings.NewReader(`{"old_password":"AuditPass#2026","new_password":"NovaSenhaForte#2026"}`))
	reqPwd.Host = "localhost:8080"
	reqPwd.Header.Set("Origin", "http://localhost:8080")
	reqPwd.Header.Set("Content-Type", "application/json")
	reqPwd.AddCookie(&http.Cookie{Name: "coorpdns_session", Value: sess1})
	rrPwd := httptest.NewRecorder()
	handler.ServeHTTP(rrPwd, reqPwd)
	if rrPwd.Code != http.StatusOK {
		t.Fatalf("troca de senha válida falhou (%d): %s", rrPwd.Code, rrPwd.Body.String())
	}

	if _, err := store.ValidateSession(sess2); err == nil {
		t.Errorf("sessão paralela sess2 deveria ter sido revogada após troca de senha")
	}
}

func TestSecurity_CSVFormulaSanitizationAndBootstrapIntegrity(t *testing.T) {
	// 1. Sanitização de fórmulas maliciosas em células CSV
	dangerous := []string{
		"=CMD|' /C calc'!A0",
		"+SUM(1,1)",
		"-2+3",
		"@SUM(A1:A2)",
		"  =HYPERLINK(\"http://evil\")",
	}
	for _, cell := range dangerous {
		got := sanitizeCSVCell(cell)
		if !strings.HasPrefix(got, "'") {
			t.Errorf("sanitizeCSVCell(%q) = %q; esperava prefixo de neutralização \"'\"", cell, got)
		}
	}

	// 2. Validação de token ativo e hash SHA-256 nos scripts de instalação remota
	_, store, handler, tmpDir := setupHardenedTestServer(t)
	origWd, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(origWd) }()

	// Cria binário fictício de agente para verificar geração de hash SHA-256
	if err := os.MkdirAll(filepath.Join(tmpDir, "downloads"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "downloads", "coorpdns-agent.exe"), []byte("dummy-signed-agent-binary"), 0600); err != nil {
		t.Fatal(err)
	}

	// Token inexistente -> 403
	reqBad := httptest.NewRequest("GET", "/install.ps1?token=tok_00000000000000000000000000000000", nil)
	reqBad.Host = "127.0.0.1:8080"
	rrBad := httptest.NewRecorder()
	handler.ServeHTTP(rrBad, reqBad)
	if rrBad.Code != http.StatusForbidden {
		t.Errorf("/install.ps1 com token inválido deveria retornar 403, obteve %d", rrBad.Code)
	}

	// Token válido e ativo -> 200 contendo verificação obrigatória Get-FileHash SHA256
	rc, err := store.CreateRemoteClient("Agente-Teste", "", "windows", 1)
	if err != nil {
		t.Fatal(err)
	}
	reqOK := httptest.NewRequest("GET", "/install.ps1?token="+rc.AuthToken, nil)
	reqOK.Host = "127.0.0.1:8080"
	rrOK := httptest.NewRecorder()
	handler.ServeHTTP(rrOK, reqOK)
	if rrOK.Code != http.StatusOK {
		t.Fatalf("/install.ps1 com token válido falhou (%d): %s", rrOK.Code, rrOK.Body.String())
	}
	script := rrOK.Body.String()
	if !strings.Contains(script, "Get-FileHash") || !strings.Contains(script, "$ExpectedSHA256") {
		t.Errorf("script /install.ps1 não contém verificação de integridade SHA-256")
	}

	// Após revogação do token -> /install.ps1 deve bloquear imediatamente (403)
	_ = store.RevokeRemoteClient(rc.AuthToken)
	reqRev := httptest.NewRequest("GET", "/install.ps1?token="+rc.AuthToken, nil)
	reqRev.Host = "127.0.0.1:8080"
	rrRev := httptest.NewRecorder()
	handler.ServeHTTP(rrRev, reqRev)
	if rrRev.Code != http.StatusForbidden {
		t.Errorf("/install.ps1 com token revogado deveria retornar 403, obteve %d", rrRev.Code)
	}

	// Verifica também sinalizador default_password_active no login inicial do admin
	loginBody, _ := json.Marshal(map[string]string{"username": "admin", "password": "admin"})
	reqLogin := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(loginBody))
	reqLogin.Host = "localhost:8080"
	reqLogin.Header.Set("Content-Type", "application/json")
	rrLogin := httptest.NewRecorder()
	handler.ServeHTTP(rrLogin, reqLogin)
	if rrLogin.Code != http.StatusOK || !strings.Contains(rrLogin.Body.String(), `"default_password_active":true`) {
		t.Errorf("esperava aviso default_password_active:true no login padrão, obteve %d: %s", rrLogin.Code, rrLogin.Body.String())
	}
}

func TestSecurity_DefaultGroupScheduleDoesNotBypassBlocking(t *testing.T) {
	srv, store, _, _ := setupHardenedTestServer(t)
	srv.filter.LoadRules(nil, []string{"||malware-domain.test^"}, nil)

	sched, err := store.GetGroupSchedule(1)
	if err != nil {
		t.Fatalf("erro ao consultar agendamento padrão: %v", err)
	}
	if sched.IsActive() {
		t.Errorf("agendamento padrão sem slots não deveria estar ativo (evita bypass fail-open)")
	}

	appRules, _ := store.GetGroupApplicationRules(1)
	catRules, _ := store.GetGroupCategoryRules(1)
	blocked, rule := srv.filter.CheckPolicy("malware-domain.test", appRules, catRules, sched.IsActive(), sched.Mode, false)
	if !blocked {
		t.Errorf("domínio bloqueado sofreu bypass fail-open com agendamento padrão! rule=%q", rule)
	}
}
