package storage

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// User representa um usuário autenticado no sistema CoorpDNS
type User struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	FullName  string    `json:"full_name"`
	Role      string    `json:"role"` // 'admin' ou 'operator'
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UserWithGroups estende o usuário com os IDs dos grupos permitidos
type UserWithGroups struct {
	User
	AllowedGroups []int64 `json:"allowed_groups"`
}

// Session representa uma sessão ativa com token criptográfico
type Session struct {
	Token     string    `json:"token"`
	UserID    int64     `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// HashPassword gera um hash seguro salted SHA-256 no formato salt$hash
func HashPassword(password string) string {
	salt := make([]byte, 16)
	rand.Read(salt)
	h := sha256.New()
	h.Write(salt)
	h.Write([]byte(password))
	return fmt.Sprintf("%x$%x", salt, h.Sum(nil))
}

// CheckPassword valida se a senha informada corresponde ao hash com salt
func CheckPassword(password, storedHash string) bool {
	parts := strings.Split(storedHash, "$")
	if len(parts) != 2 {
		// Fallback de compatibilidade caso salvo como texto plano inicial
		return storedHash == password
	}
	salt, err := hex.DecodeString(parts[0])
	if err != nil {
		return false
	}
	h := sha256.New()
	h.Write(salt)
	h.Write([]byte(password))
	expected := fmt.Sprintf("%x", h.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(parts[1]), []byte(expected)) == 1
}

// MigrateUsers cria as tabelas de usuários, permissões e sessões e inicializa o admin padrão
func (d *DB) MigrateUsers() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		full_name TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'operator',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS user_group_permissions (
		user_id INTEGER NOT NULL,
		group_id INTEGER NOT NULL,
		PRIMARY KEY (user_id, group_id),
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (group_id) REFERENCES client_groups(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS sessions (
		token TEXT PRIMARY KEY,
		user_id INTEGER NOT NULL,
		expires_at DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);
	`
	if _, err := d.db.Exec(schema); err != nil {
		return err
	}

	// Cria o usuário padrão 'admin' com senha 'admin' se não existir nenhum usuário
	var count int
	if err := d.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count); err == nil && count == 0 {
		adminHash := HashPassword("admin")
		d.db.Exec(`
			INSERT INTO users (id, username, password_hash, full_name, role) 
			VALUES (1, 'admin', ?, 'Administrador Principal', 'admin')
		`, adminHash)
	}

	return nil
}

// AuthenticateUser valida credenciais e retorna o usuário com seus grupos autorizados
func (d *DB) AuthenticateUser(username, password string) (*UserWithGroups, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil, errors.New("usuário e senha são obrigatórios")
	}

	var u UserWithGroups
	var hash string
	query := "SELECT id, username, password_hash, full_name, role, created_at, updated_at FROM users WHERE username = ?"
	err := d.db.QueryRow(query, username).Scan(&u.ID, &u.Username, &hash, &u.FullName, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("usuário ou senha incorretos")
		}
		return nil, err
	}

	if !CheckPassword(password, hash) {
		return nil, errors.New("usuário ou senha incorretos")
	}

	// Carrega permissões de grupos
	groups, err := d.GetUserGroupIDs(u.ID, u.Role)
	if err != nil {
		return nil, err
	}
	u.AllowedGroups = groups

	return &u, nil
}

// CreateSession cria um token de sessão de 30 dias para o usuário autenticado
func (d *DB) CreateSession(userID int64, duration time.Duration) (string, error) {
	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	token := "ses_" + hex.EncodeToString(tokenBytes)
	expiresAt := time.Now().Add(duration)

	// Remove sessões expiradas antes de criar uma nova
	d.db.Exec("DELETE FROM sessions WHERE expires_at < datetime('now')")

	query := "INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)"
	_, err := d.db.Exec(query, token, userID, expiresAt)
	if err != nil {
		return "", err
	}
	return token, nil
}

// ValidateSession verifica se um token de sessão é válido e retorna o usuário com grupos
func (d *DB) ValidateSession(token string) (*UserWithGroups, error) {
	if token == "" {
		return nil, errors.New("token ausente")
	}

	query := `
		SELECT u.id, u.username, u.full_name, u.role, u.created_at, u.updated_at
		FROM sessions s
		JOIN users u ON s.user_id = u.id
		WHERE s.token = ? AND s.expires_at > datetime('now')
	`
	var u UserWithGroups
	err := d.db.QueryRow(query, token).Scan(&u.ID, &u.Username, &u.FullName, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("sessão inválida ou expirada")
		}
		return nil, err
	}

	groups, err := d.GetUserGroupIDs(u.ID, u.Role)
	if err != nil {
		return nil, err
	}
	u.AllowedGroups = groups

	return &u, nil
}

// DeleteSession remove o token de sessão (logout)
func (d *DB) DeleteSession(token string) error {
	_, err := d.db.Exec("DELETE FROM sessions WHERE token = ?", token)
	return err
}

// GetUserGroupIDs retorna a lista de IDs dos grupos aos quais o usuário tem acesso
func (d *DB) GetUserGroupIDs(userID int64, role string) ([]int64, error) {
	// Administradores têm acesso a TODOS os grupos do sistema
	if role == "admin" {
		rows, err := d.db.Query("SELECT id FROM client_groups ORDER BY id ASC")
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err == nil {
				ids = append(ids, id)
			}
		}
		return ids, nil
	}

	// Operadores têm apenas os grupos expressamente autorizados
	rows, err := d.db.Query("SELECT group_id FROM user_group_permissions WHERE user_id = ? ORDER BY group_id ASC", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// ListUsers lista todos os usuários cadastrados e os grupos que cada um gerencia
func (d *DB) ListUsers() ([]UserWithGroups, error) {
	query := "SELECT id, username, full_name, role, created_at, updated_at FROM users ORDER BY id ASC"
	rows, err := d.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []UserWithGroups
	for rows.Next() {
		var u UserWithGroups
		if err := rows.Scan(&u.ID, &u.Username, &u.FullName, &u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
			continue
		}
		groups, _ := d.GetUserGroupIDs(u.ID, u.Role)
		u.AllowedGroups = groups
		users = append(users, u)
	}
	return users, nil
}

// CreateUser cria um novo usuário no sistema vinculando os grupos permitidos
func (d *DB) CreateUser(username, password, fullName, role string, groupIDs []int64) (*UserWithGroups, error) {
	username = strings.TrimSpace(username)
	fullName = strings.TrimSpace(fullName)
	if username == "" {
		return nil, errors.New("o login/usuário é obrigatório")
	}
	if password == "" {
		return nil, errors.New("a senha é obrigatória")
	}
	if fullName == "" {
		fullName = username
	}
	if role != "admin" {
		role = "operator"
	}

	hash := HashPassword(password)
	tx, err := d.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	res, err := tx.Exec("INSERT INTO users (username, password_hash, full_name, role) VALUES (?, ?, ?, ?)", username, hash, fullName, role)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, errors.New("já existe um usuário cadastrado com este login")
		}
		return nil, err
	}

	userID, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	// Se for operador, salva permissões dos grupos
	if role == "operator" {
		stmt, err := tx.Prepare("INSERT INTO user_group_permissions (user_id, group_id) VALUES (?, ?)")
		if err != nil {
			return nil, err
		}
		defer stmt.Close()

		for _, gid := range groupIDs {
			stmt.Exec(userID, gid)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return d.GetUserByID(userID)
}

// GetUserByID busca usuário completo pelo ID
func (d *DB) GetUserByID(id int64) (*UserWithGroups, error) {
	var u UserWithGroups
	query := "SELECT id, username, full_name, role, created_at, updated_at FROM users WHERE id = ?"
	err := d.db.QueryRow(query, id).Scan(&u.ID, &u.Username, &u.FullName, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}

	groups, err := d.GetUserGroupIDs(u.ID, u.Role)
	if err != nil {
		return nil, err
	}
	u.AllowedGroups = groups

	return &u, nil
}

// UpdateUser atualiza os dados cadastrais, grupos permitidos e opcionalmente a senha
func (d *DB) UpdateUser(id int64, username, password, fullName, role string, groupIDs []int64) error {
	username = strings.TrimSpace(username)
	fullName = strings.TrimSpace(fullName)
	if username == "" {
		return errors.New("o login/usuário é obrigatório")
	}
	if role != "admin" {
		role = "operator"
	}

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if password != "" {
		hash := HashPassword(password)
		_, err = tx.Exec("UPDATE users SET username = ?, password_hash = ?, full_name = ?, role = ?, updated_at = datetime('now') WHERE id = ?",
			username, hash, fullName, role, id)
	} else {
		_, err = tx.Exec("UPDATE users SET username = ?, full_name = ?, role = ?, updated_at = datetime('now') WHERE id = ?",
			username, fullName, role, id)
	}
	if err != nil {
		return err
	}

	// Atualiza permissões de grupos
	tx.Exec("DELETE FROM user_group_permissions WHERE user_id = ?", id)
	if role == "operator" {
		stmt, err := tx.Prepare("INSERT INTO user_group_permissions (user_id, group_id) VALUES (?, ?)")
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, gid := range groupIDs {
			stmt.Exec(id, gid)
		}
	}

	return tx.Commit()
}

// DeleteUser remove um usuário do sistema
func (d *DB) DeleteUser(id int64) error {
	if id == 1 {
		return errors.New("não é permitido excluir o administrador principal (ID 1)")
	}
	_, err := d.db.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}

// ChangePassword permite ao usuário alterar sua própria senha
func (d *DB) ChangePassword(userID int64, oldPassword, newPassword string) error {
	if newPassword == "" {
		return errors.New("a nova senha não pode ser vazia")
	}

	var storedHash string
	err := d.db.QueryRow("SELECT password_hash FROM users WHERE id = ?", userID).Scan(&storedHash)
	if err != nil {
		return errors.New("usuário não encontrado")
	}

	if !CheckPassword(oldPassword, storedHash) {
		return errors.New("senha atual incorreta")
	}

	newHash := HashPassword(newPassword)
	_, err = d.db.Exec("UPDATE users SET password_hash = ?, updated_at = datetime('now') WHERE id = ?", newHash, userID)
	return err
}

// buildGroupFilterClause gera uma cláusula IN (?) para filtrar IDs de grupos
func buildGroupFilterClause(column string, groupIDs []int64) (string, []interface{}) {
	if len(groupIDs) == 0 {
		return "1=0", nil
	}
	placeholders := make([]string, len(groupIDs))
	args := make([]interface{}, len(groupIDs))
	for i, id := range groupIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	return fmt.Sprintf("%s IN (%s)", column, strings.Join(placeholders, ",")), args
}

// buildClientInGroupSubquery gera a subquery para filtrar consultas DNS de clientes que pertencem aos grupos permitidos
func buildClientInGroupSubquery(column string, groupIDs []int64) (string, []interface{}) {
	if len(groupIDs) == 0 {
		return "1=0", nil
	}
	placeholders := make([]string, len(groupIDs))
	args := make([]interface{}, len(groupIDs))
	for i, id := range groupIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	inClause := strings.Join(placeholders, ",")

	cond := fmt.Sprintf(`%s IN (
		SELECT ip FROM clients WHERE group_id IN (%s) AND status = 'active'
		UNION
		SELECT auth_token FROM clients WHERE group_id IN (%s) AND status = 'active' AND auth_token != ''
		UNION
		SELECT 'remote-' || substr(auth_token, 1, 10) FROM clients WHERE group_id IN (%s) AND status = 'active' AND auth_token != ''
	)`, column, inClause, inClause, inClause)

	var allArgs []interface{}
	allArgs = append(allArgs, args...)
	allArgs = append(allArgs, args...)
	allArgs = append(allArgs, args...)
	return cond, allArgs
}
