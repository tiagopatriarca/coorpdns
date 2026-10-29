package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGroupDomainsAndAuditStorage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "coorpdns-test-*")
	if err != nil {
		t.Fatalf("Erro ao criar temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test_coorpdns.db")
	db, err := NewDB(dbPath, 30, 7)
	if err != nil {
		t.Fatalf("Erro ao criar DB: %v", err)
	}
	defer db.Close()

	// 1. Testar Domínios de Grupo
	err = db.UpsertGroupDomainRule(1, "tiktok.com", "block")
	if err != nil {
		t.Fatalf("Erro ao inserir domínio de grupo: %v", err)
	}
	err = db.UpsertGroupDomainRule(1, "intranet.local", "allow")
	if err != nil {
		t.Fatalf("Erro ao inserir allow domínio de grupo: %v", err)
	}

	rules, err := db.GetGroupDomainRules(1)
	if err != nil {
		t.Fatalf("Erro ao buscar domínios de grupo: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("Esperado 2 regras de domínio, obteve %d", len(rules))
	}

	rulesMap, err := db.GetGroupDomainRulesMap(1)
	if err != nil || rulesMap["tiktok.com"] != "block" || rulesMap["intranet.local"] != "allow" {
		t.Fatalf("Mapeamento de domínios incorreto: %v", rulesMap)
	}

	// 2. Testar Auditoria de Grupo (máximo 10 registros mais recentes)
	for i := 1; i <= 15; i++ {
		db.AddGroupAuditLog(1, "admin", "Administrador", "test", "Evento de teste")
	}

	logs, err := db.GetGroupAuditLogs(1, 10)
	if err != nil {
		t.Fatalf("Erro ao buscar auditoria de grupo: %v", err)
	}
	if len(logs) != 10 {
		t.Fatalf("Esperado exatamente 10 registros de auditoria, obteve %d", len(logs))
	}

	// 3. Testar GetDashboardGroupStats excluindo grupos sem dispositivos
	stats, err := db.GetDashboardGroupStats(7, nil)
	if err != nil {
		t.Fatalf("Erro ao buscar dashboard group stats: %v", err)
	}
	// Nenhum dispositivo cadastrado ainda -> deve retornar vazio
	if len(stats) != 0 {
		t.Fatalf("Esperava 0 grupos com dispositivos cadastrados, obteve %d", len(stats))
	}

	// 4. Testar GetGroupReport com fatias de categorias e apps inicializadas
	rep, err := db.GetGroupReport(1, 7, 50)
	if err != nil {
		t.Fatalf("Erro ao buscar relatório de grupo: %v", err)
	}
	if rep.CategoriesStats == nil || rep.ApplicationsStats == nil {
		t.Fatalf("CategoriesStats ou ApplicationsStats não devem ser nil")
	}
}
