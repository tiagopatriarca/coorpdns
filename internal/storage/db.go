package storage

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type QueryLogRecord struct {
	Timestamp   time.Time `json:"timestamp"`
	ClientIP    string    `json:"client_ip"`
	Domain      string    `json:"domain"`
	QType       string    `json:"qtype"`
	Status      string    `json:"status"` // "allowed", "blocked"
	RuleMatched string    `json:"rule_matched"`
	Upstream    string    `json:"upstream"`
	LatencyMs   float64   `json:"latency_ms"`
}

type ClientGroup struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Color       string    `json:"color"`
	ClientCount int       `json:"client_count"`
	CreatedAt   time.Time `json:"created_at"`
}

type Client struct {
	IP           string    `json:"ip"`
	Name         string    `json:"name"`
	MAC          string    `json:"mac"`
	Comment      string    `json:"comment"`
	BlockEnabled bool      `json:"block_enabled"`
	AuthToken    string    `json:"auth_token"`    // Token secreto exclusivo para clientes remotos
	IsRemote     bool      `json:"is_remote"`     // True se for cliente remoto autenticado por token
	Status       string    `json:"status"`        // "active", "revoked"
	GroupID      int64     `json:"group_id"`      // ID do grupo ao qual o dispositivo pertence
	GroupName    string    `json:"group_name"`    // Nome do grupo para exibição
	GroupColor   string    `json:"group_color"`   // Cor do grupo para badge visual
	CreatedAt    time.Time `json:"created_at"`
}

type ConditionalForwarder struct {
	ID         int64     `json:"id"`
	Domain     string    `json:"domain"`
	UpstreamIP string    `json:"upstream_ip"`
	Comment    string    `json:"comment"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
}

type DB struct {
	db          *sql.DB
	logChan     chan *QueryLogRecord
	stopChan    chan struct{}
	wg          sync.WaitGroup
	historyDays int
	rawDays     int
}

func NewDB(dbPath string, historyDays, rawDays int) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}

	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=10000&_timeout=10000", dbPath)
	sqlDB, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}

	// SQLite em WAL mode suporta múltiplos leitores simultâneos sem contenção
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(10 * time.Minute)

	d := &DB{
		db:          sqlDB,
		logChan:     make(chan *QueryLogRecord, 10000),
		stopChan:    make(chan struct{}),
		historyDays: historyDays,
		rawDays:     rawDays,
	}

	if err := d.migrate(); err != nil {
		sqlDB.Close()
		return nil, err
	}

	d.wg.Add(1)
	go d.flushWorker()

	// Inicia rotina diária de agregação e limpeza
	go d.scheduleMaintenance()

	return d, nil
}

func (d *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS dns_queries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME NOT NULL,
		client_ip TEXT NOT NULL,
		domain TEXT NOT NULL,
		qtype TEXT NOT NULL,
		status TEXT NOT NULL,
		rule_matched TEXT,
		upstream TEXT,
		latency_ms REAL
	);
	CREATE INDEX IF NOT EXISTS idx_queries_ts ON dns_queries(timestamp);
	CREATE INDEX IF NOT EXISTS idx_queries_client ON dns_queries(client_ip);
	CREATE INDEX IF NOT EXISTS idx_queries_domain ON dns_queries(domain);

	CREATE TABLE IF NOT EXISTS daily_client_domain_stats (
		date TEXT NOT NULL,
		client_ip TEXT NOT NULL,
		domain TEXT NOT NULL,
		total_queries INTEGER NOT NULL DEFAULT 0,
		blocked_queries INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (date, client_ip, domain)
	);
	CREATE INDEX IF NOT EXISTS idx_stats_date ON daily_client_domain_stats(date);
	CREATE INDEX IF NOT EXISTS idx_stats_client_date ON daily_client_domain_stats(client_ip, date);
	CREATE INDEX IF NOT EXISTS idx_stats_domain_date ON daily_client_domain_stats(domain, date);

	CREATE TABLE IF NOT EXISTS client_groups (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		description TEXT,
		color TEXT DEFAULT '#10b981',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	INSERT OR IGNORE INTO client_groups (id, name, description, color) VALUES
	(1, 'Geral / Padrão', 'Dispositivos não categorizados', '#64748b'),
	(2, 'Smart TVs & Streaming', 'TVs, consoles e reprodutores de mídia', '#38bdf8'),
	(3, 'Crianças / Família', 'Celulares e tablets com controle parental', '#f59e0b'),
	(4, 'Trabalho / Home Office', 'Computadores e notebooks corporativos', '#10b981'),
	(5, 'Dispositivos IoT & Casa Inteligente', 'Lâmpadas, tomadas e sensores', '#a855f7');

	CREATE TABLE IF NOT EXISTS clients (
		ip TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		mac_address TEXT,
		comment TEXT,
		block_enabled INTEGER DEFAULT 1,
		auth_token TEXT UNIQUE,
		is_remote INTEGER DEFAULT 0,
		status TEXT DEFAULT 'active',
		group_id INTEGER DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_clients_token ON clients(auth_token);
	CREATE INDEX IF NOT EXISTS idx_clients_group ON clients(group_id);

	CREATE TABLE IF NOT EXISTS group_category_rules (
		group_id INTEGER NOT NULL,
		category_id TEXT NOT NULL,
		action TEXT NOT NULL DEFAULT 'block',
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (group_id, category_id),
		FOREIGN KEY (group_id) REFERENCES client_groups(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS conditional_forwarders (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		domain TEXT NOT NULL UNIQUE,
		upstream_ip TEXT NOT NULL,
		comment TEXT,
		enabled INTEGER DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	-- Regras padrão para grupos iniciais
	INSERT OR IGNORE INTO group_category_rules (group_id, category_id, action) VALUES
	(1, 'ads', 'block'), (1, 'trackers', 'block'), (1, 'adult', 'block'),
	(2, 'ads', 'block'), (2, 'trackers', 'block'), (2, 'adult', 'block'), (2, 'gambling', 'block'), (2, 'dating', 'block'), (2, 'p2p_torrent', 'block'), (2, 'gaming', 'block'),
	(3, 'ads', 'block'), (3, 'trackers', 'block'), (3, 'adult', 'block'), (3, 'gambling', 'block'), (3, 'p2p_torrent', 'block'), (3, 'social', 'block'),
	(4, 'ads', 'block'), (4, 'trackers', 'block'), (4, 'adult', 'block'), (4, 'gambling', 'block'), (4, 'p2p_torrent', 'block');
	`
	if _, err := d.db.Exec(schema); err != nil {
		return err
	}

	// Migração retroativa de colunas se tabela já existia
	d.db.Exec("ALTER TABLE clients ADD COLUMN auth_token TEXT UNIQUE")
	d.db.Exec("ALTER TABLE clients ADD COLUMN is_remote INTEGER DEFAULT 0")
	d.db.Exec("ALTER TABLE clients ADD COLUMN status TEXT DEFAULT 'active'")
	d.db.Exec("ALTER TABLE clients ADD COLUMN group_id INTEGER DEFAULT 1")

	return nil
}

func (d *DB) LogQuery(record *QueryLogRecord) {
	select {
	case d.logChan <- record:
	default:
		// Se o buffer estiver cheio, descarta para não travar resoluções DNS em tempo real
	}
}

func (d *DB) flushWorker() {
	defer d.wg.Done()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var batch []*QueryLogRecord

	flush := func() {
		if len(batch) == 0 {
			return
		}

		tx, err := d.db.Begin()
		if err != nil {
			batch = batch[:0]
			return
		}

		stmtQuery, err := tx.Prepare(`
			INSERT INTO dns_queries (timestamp, client_ip, domain, qtype, status, rule_matched, upstream, latency_ms)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			tx.Rollback()
			batch = batch[:0]
			return
		}
		defer stmtQuery.Close()

		stmtDaily, err := tx.Prepare(`
			INSERT INTO daily_client_domain_stats (date, client_ip, domain, total_queries, blocked_queries)
			VALUES (?, ?, ?, 1, ?)
			ON CONFLICT(date, client_ip, domain) DO UPDATE SET
				total_queries = total_queries + 1,
				blocked_queries = blocked_queries + excluded.blocked_queries
		`)
		if err != nil {
			tx.Rollback()
			batch = batch[:0]
			return
		}
		defer stmtDaily.Close()

		for _, r := range batch {
			stmtQuery.Exec(r.Timestamp, r.ClientIP, r.Domain, r.QType, r.Status, r.RuleMatched, r.Upstream, r.LatencyMs)

			isBlocked := 0
			if r.Status == "blocked" {
				isBlocked = 1
			}
			dateStr := r.Timestamp.Format("2006-01-02")
			stmtDaily.Exec(dateStr, r.ClientIP, r.Domain, isBlocked)
		}

		tx.Commit()
		batch = batch[:0]
	}

	for {
		select {
		case r := <-d.logChan:
			batch = append(batch, r)
			if len(batch) >= 150 {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-d.stopChan:
			// Drena os restantes
			for {
				select {
				case r := <-d.logChan:
					batch = append(batch, r)
				default:
					flush()
					return
				}
			}
		}
	}
}

func (d *DB) scheduleMaintenance() {
	ticker := time.NewTicker(12 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.RunCleanup()
		case <-d.stopChan:
			return
		}
	}
}

func (d *DB) RunCleanup() {
	rawCutoff := time.Now().AddDate(0, 0, -d.rawDays)
	d.db.Exec("DELETE FROM dns_queries WHERE timestamp < ?", rawCutoff)

	historyCutoff := time.Now().AddDate(0, 0, -d.historyDays).Format("2006-01-02")
	d.db.Exec("DELETE FROM daily_client_domain_stats WHERE date < ?", historyCutoff)

	// Otimiza páginas do SQLite
	d.db.Exec("PRAGMA optimize")
}

func (d *DB) Close() error {
	close(d.stopChan)
	d.wg.Wait()
	return d.db.Close()
}

// Clientes
func (d *DB) UpsertClient(c *Client) error {
	query := `
		INSERT INTO clients (ip, name, mac_address, comment, block_enabled, auth_token, is_remote, status, group_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(ip) DO UPDATE SET
			name = excluded.name,
			mac_address = excluded.mac_address,
			comment = excluded.comment,
			block_enabled = excluded.block_enabled,
			auth_token = COALESCE(NULLIF(excluded.auth_token, ''), clients.auth_token),
			is_remote = CASE WHEN clients.is_remote = 1 THEN 1 ELSE excluded.is_remote END,
			status = excluded.status,
			group_id = COALESCE(excluded.group_id, clients.group_id)
	`
	blockVal := 1
	if !c.BlockEnabled {
		blockVal = 0
	}
	isRemoteVal := 0
	if c.IsRemote {
		isRemoteVal = 1
	}
	if c.Status == "" {
		c.Status = "active"
	}
	if c.GroupID <= 0 {
		c.GroupID = 1
	}
	_, err := d.db.Exec(query, c.IP, c.Name, c.MAC, c.Comment, blockVal, c.AuthToken, isRemoteVal, c.Status, c.GroupID)
	return err
}

func (d *DB) GetClients() ([]Client, error) {
	query := `
		SELECT 
			c.ip, c.name, c.mac_address, c.comment, c.block_enabled, c.auth_token, c.is_remote, c.status, 
			COALESCE(c.group_id, 1), COALESCE(g.name, 'Geral / Padrão'), COALESCE(g.color, '#64748b'), c.created_at 
		FROM clients c
		LEFT JOIN client_groups g ON c.group_id = g.id
		ORDER BY c.is_remote DESC, c.name ASC
	`
	rows, err := d.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Client
	for rows.Next() {
		var c Client
		var blockVal, isRemoteVal int
		var mac, comment, token, status sql.NullString
		if err := rows.Scan(&c.IP, &c.Name, &mac, &comment, &blockVal, &token, &isRemoteVal, &status, &c.GroupID, &c.GroupName, &c.GroupColor, &c.CreatedAt); err != nil {
			continue
		}
		c.MAC = mac.String
		c.Comment = comment.String
		c.AuthToken = token.String
		c.BlockEnabled = blockVal == 1
		c.IsRemote = isRemoteVal == 1
		c.Status = status.String
		if c.Status == "" {
			c.Status = "active"
		}
		list = append(list, c)
	}
	return list, nil
}

func (d *DB) GetClientByToken(token string) (*Client, error) {
	if token == "" {
		return nil, fmt.Errorf("token vazio")
	}

	row := d.db.QueryRow(`
		SELECT 
			c.ip, c.name, c.mac_address, c.comment, c.block_enabled, c.auth_token, c.is_remote, c.status,
			COALESCE(c.group_id, 1), COALESCE(g.name, 'Geral / Padrão'), COALESCE(g.color, '#64748b'), c.created_at
		FROM clients c
		LEFT JOIN client_groups g ON c.group_id = g.id
		WHERE c.auth_token = ? AND c.status = 'active'
	`, token)

	var c Client
	var blockVal, isRemoteVal int
	var mac, comment, tok, status sql.NullString
	if err := row.Scan(&c.IP, &c.Name, &mac, &comment, &blockVal, &tok, &isRemoteVal, &status, &c.GroupID, &c.GroupName, &c.GroupColor, &c.CreatedAt); err != nil {
		return nil, err
	}
	c.MAC = mac.String
	c.Comment = comment.String
	c.AuthToken = tok.String
	c.BlockEnabled = blockVal == 1
	c.IsRemote = isRemoteVal == 1
	c.Status = status.String
	return &c, nil
}

func (d *DB) CreateRemoteClient(name, comment string, groupID int64) (*Client, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	token := "tok_" + hex.EncodeToString(b)
	virtualIP := "remote-" + token[:10]

	if groupID <= 0 {
		groupID = 1
	}

	client := &Client{
		IP:           virtualIP,
		Name:         name,
		Comment:      comment,
		BlockEnabled: true,
		AuthToken:    token,
		IsRemote:     true,
		Status:       "active",
		GroupID:      groupID,
		CreatedAt:    time.Now(),
	}

	if err := d.UpsertClient(client); err != nil {
		return nil, err
	}
	return client, nil
}

func (d *DB) RevokeRemoteClient(token string) error {
	_, err := d.db.Exec("UPDATE clients SET status = 'revoked' WHERE auth_token = ?", token)
	return err
}

func (d *DB) AssignClientToGroup(clientIP string, groupID int64) error {
	_, err := d.db.Exec("UPDATE clients SET group_id = ? WHERE ip = ?", groupID, clientIP)
	return err
}

// Operações de Grupos
func (d *DB) GetGroups() ([]ClientGroup, error) {
	query := `
		SELECT 
			g.id, g.name, COALESCE(g.description, ''), g.color, g.created_at,
			COUNT(c.ip) AS client_count
		FROM client_groups g
		LEFT JOIN clients c ON g.id = c.group_id AND c.status = 'active'
		GROUP BY g.id
		ORDER BY g.id ASC
	`
	rows, err := d.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []ClientGroup
	for rows.Next() {
		var g ClientGroup
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.Color, &g.CreatedAt, &g.ClientCount); err != nil {
			continue
		}
		groups = append(groups, g)
	}
	return groups, nil
}

func (d *DB) CreateGroup(name, description, color string) (*ClientGroup, error) {
	if color == "" {
		color = "#10b981"
	}
	res, err := d.db.Exec("INSERT INTO client_groups (name, description, color) VALUES (?, ?, ?)", name, description, color)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &ClientGroup{
		ID:          id,
		Name:        name,
		Description: description,
		Color:       color,
		CreatedAt:   time.Now(),
	}, nil
}

func (d *DB) UpdateGroup(id int64, name, description, color string) error {
	_, err := d.db.Exec("UPDATE client_groups SET name = ?, description = ?, color = ? WHERE id = ?", name, description, color, id)
	return err
}

func (d *DB) DeleteGroup(id int64) error {
	if id == 1 {
		return fmt.Errorf("o grupo padrão não pode ser excluído")
	}
	// Move dispositivos pertencentes ao grupo excluído para o grupo 1 (Padrão)
	d.db.Exec("UPDATE clients SET group_id = 1 WHERE group_id = ?", id)
	_, err := d.db.Exec("DELETE FROM client_groups WHERE id = ?", id)
	return err
}

func (d *DB) GetRecentLogs(limit int, filterClient, filterDomain, filterStatus string) ([]QueryLogRecord, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	query := "SELECT timestamp, client_ip, domain, qtype, status, rule_matched, upstream, latency_ms FROM dns_queries WHERE 1=1"
	var args []interface{}

	if filterClient != "" {
		query += " AND client_ip = ?"
		args = append(args, filterClient)
	}
	if filterDomain != "" {
		query += " AND domain LIKE ?"
		args = append(args, "%"+filterDomain+"%")
	}
	if filterStatus != "" && filterStatus != "all" {
		query += " AND status = ?"
		args = append(args, filterStatus)
	}

	query += " ORDER BY timestamp DESC LIMIT ?"
	args = append(args, limit)

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []QueryLogRecord
	for rows.Next() {
		var r QueryLogRecord
		var rule, up sql.NullString
		if err := rows.Scan(&r.Timestamp, &r.ClientIP, &r.Domain, &r.QType, &r.Status, &rule, &up, &r.LatencyMs); err != nil {
			continue
		}
		r.RuleMatched = rule.String
		r.Upstream = up.String
		logs = append(logs, r)
	}
	return logs, nil
}

// Retorna as categorias e ações ('block' ou 'allow') configuradas para um grupo
func (d *DB) GetGroupCategoryRules(groupID int64) (map[string]string, error) {
	rows, err := d.db.Query("SELECT category_id, action FROM group_category_rules WHERE group_id = ?", groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := make(map[string]string)
	for rows.Next() {
		var cat, action string
		if err := rows.Scan(&cat, &action); err == nil {
			rules[cat] = action
		}
	}
	return rules, nil
}

// Salva ou atualiza a ação de uma categoria para um grupo
func (d *DB) SetGroupCategoryRule(groupID int64, categoryID, action string) error {
	_, err := d.db.Exec(`
		INSERT INTO group_category_rules (group_id, category_id, action, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(group_id, category_id) DO UPDATE SET action = excluded.action, updated_at = CURRENT_TIMESTAMP
	`, groupID, categoryID, action)
	return err
}

// Salva múltiplas regras de categorias de uma vez para um grupo
func (d *DB) SetGroupCategories(groupID int64, rules map[string]string) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO group_category_rules (group_id, category_id, action, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(group_id, category_id) DO UPDATE SET action = excluded.action, updated_at = CURRENT_TIMESTAMP
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for cat, action := range rules {
		if _, err := stmt.Exec(groupID, cat, action); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Retorna o group_id de um cliente buscando por IP ou auth_token. Se não encontrado, retorna 1 (Padrão)
func (d *DB) GetClientGroupID(clientIPOrToken string) int64 {
	var groupID int64
	err := d.db.QueryRow("SELECT group_id FROM clients WHERE ip = ? OR auth_token = ?", clientIPOrToken, clientIPOrToken).Scan(&groupID)
	if err != nil || groupID <= 0 {
		return 1 // Grupo Padrão
	}
	return groupID
}

// Busca todos os encaminhadores condicionais (Active Directory / Zonas Locais)
func (d *DB) GetConditionalForwarders() ([]ConditionalForwarder, error) {
	rows, err := d.db.Query("SELECT id, domain, upstream_ip, comment, enabled, created_at FROM conditional_forwarders ORDER BY domain ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []ConditionalForwarder
	for rows.Next() {
		var f ConditionalForwarder
		var comment sql.NullString
		if err := rows.Scan(&f.ID, &f.Domain, &f.UpstreamIP, &comment, &f.Enabled, &f.CreatedAt); err != nil {
			continue
		}
		f.Comment = comment.String
		list = append(list, f)
	}
	return list, nil
}

// Cria um novo encaminhador condicional (ex: empresa.local -> 192.168.1.10)
func (d *DB) CreateConditionalForwarder(domain, upstreamIP, comment string) (*ConditionalForwarder, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	domain = strings.TrimSuffix(domain, ".")

	res, err := d.db.Exec("INSERT INTO conditional_forwarders (domain, upstream_ip, comment, enabled) VALUES (?, ?, ?, 1)", domain, upstreamIP, comment)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &ConditionalForwarder{
		ID:         id,
		Domain:     domain,
		UpstreamIP: upstreamIP,
		Comment:    comment,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}, nil
}

// Remove um encaminhador condicional
func (d *DB) DeleteConditionalForwarder(id int64) error {
	_, err := d.db.Exec("DELETE FROM conditional_forwarders WHERE id = ?", id)
	return err
}
