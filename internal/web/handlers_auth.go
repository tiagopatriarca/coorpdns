package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coorpdns/coorpdns/internal/storage"
)

func (s *Server) extractSessionToken(r *http.Request) (token string, fromCookie bool) {
	// 1. Tenta obter do Cookie HttpOnly
	if cookie, err := r.Cookie("coorpdns_session"); err == nil && cookie != nil && cookie.Value != "" {
		return strings.TrimSpace(cookie.Value), true
	}
	// 2. Tenta obter do cabeçalho Authorization: Bearer <token>
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer ")), false
	}
	// Nota de Segurança: tokens de sessão na query string (?session=...) são proibidos para evitar vazamento em logs/Referer
	return "", false
}

func (s *Server) getAuthUser(r *http.Request) *storage.UserWithGroups {
	token, _ := s.extractSessionToken(r)
	if token == "" {
		return nil
	}

	u, err := s.storage.ValidateSession(token)
	if err != nil {
		return nil
	}
	return u
}

func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) *storage.UserWithGroups {
	u := s.getAuthUser(r)
	if u == nil {
		http.Error(w, "Não autorizado. Por favor realize o login.", http.StatusUnauthorized)
		return nil
	}
	return u
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) *storage.UserWithGroups {
	u := s.requireAuth(w, r)
	if u == nil {
		return nil
	}
	if u.Role != "admin" {
		http.Error(w, "Acesso negado. Ação restrita a administradores do sistema.", http.StatusForbidden)
		return nil
	}
	return u
}

func containsInt64(slice []int64, val int64) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

// POST /api/v1/auth/login
func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clientIP := s.clientIP(r)
	ipKey := "ip:" + clientIP
	if s.loginLimiter != nil && !s.loginLimiter.Allow(ipKey) {
		http.Error(w, "Muitas tentativas de login incorretas. Por segurança, aguarde 5 minutos antes de tentar novamente.", http.StatusTooManyRequests)
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Dados de login inválidos", http.StatusBadRequest)
		return
	}

	normUser := strings.ToLower(strings.TrimSpace(req.Username))
	userKey := ""
	if normUser != "" {
		userKey = "user:" + normUser
		if s.loginLimiter != nil && !s.loginLimiter.Allow(userKey) {
			http.Error(w, "Conta temporariamente protegida contra força bruta. Aguarde 5 minutos antes de tentar novamente.", http.StatusTooManyRequests)
			return
		}
	}

	user, err := s.storage.AuthenticateUser(req.Username, req.Password)
	if err != nil {
		if s.loginLimiter != nil {
			s.loginLimiter.RecordFailedAttempt(ipKey)
			if userKey != "" {
				s.loginLimiter.RecordFailedAttempt(userKey)
			}
		}
		http.Error(w, "Usuário ou senha incorretos", http.StatusUnauthorized)
		return
	}

	if s.loginLimiter != nil {
		s.loginLimiter.Reset(ipKey)
		if userKey != "" {
			s.loginLimiter.Reset(userKey)
		}
	}

	token, err := s.storage.CreateSession(user.ID, 30*time.Minute)
	if err != nil {
		http.Error(w, "Erro interno ao iniciar sessão", http.StatusInternalServerError)
		return
	}

	user.DefaultPasswordActive = s.storage.IsUsingDefaultPassword(user.ID)
	isSecure := s.isSecure(r)

	http.SetCookie(w, &http.Cookie{
		Name:     "coorpdns_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
	})

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"token": token,
		"user":  user,
	})
}

// POST /api/v1/auth/logout
func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token, _ := s.extractSessionToken(r)
	if token != "" {
		_ = s.storage.DeleteSession(token)
	}

	isSecure := s.isSecure(r)
	http.SetCookie(w, &http.Cookie{
		Name:     "coorpdns_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	jsonResponse(w, http.StatusOK, map[string]string{
		"message": "Sessão finalizada com sucesso",
	})
}

// GET /api/v1/auth/me
func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	u := s.getAuthUser(r)
	if u == nil {
		http.Error(w, "Não autenticado", http.StatusUnauthorized)
		return
	}
	u.DefaultPasswordActive = s.storage.IsUsingDefaultPassword(u.ID)
	jsonResponse(w, http.StatusOK, u)
}

// POST /api/v1/auth/password
func (s *Server) handleAuthChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	u := s.requireAuth(w, r)
	if u == nil {
		return
	}

	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Corpo inválido", http.StatusBadRequest)
		return
	}

	if len(req.NewPassword) < 8 {
		http.Error(w, "A nova senha deve possuir no mínimo 8 caracteres", http.StatusBadRequest)
		return
	}

	currToken, _ := s.extractSessionToken(r)
	if err := s.storage.ChangePassword(u.ID, req.OldPassword, req.NewPassword, currToken); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	jsonResponse(w, http.StatusOK, map[string]string{"message": "Senha alterada com sucesso"})
}

// GET /api/v1/users & POST /api/v1/users (Admin Only)
func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	admin := s.requireAdmin(w, r)
	if admin == nil {
		return
	}

	switch r.Method {
	case http.MethodGet:
		users, err := s.storage.ListUsers()
		if err != nil {
			http.Error(w, "Erro ao listar usuários", http.StatusInternalServerError)
			return
		}
		jsonResponse(w, http.StatusOK, users)

	case http.MethodPost:
		var req struct {
			Username string  `json:"username"`
			Password string  `json:"password"`
			FullName string  `json:"full_name"`
			Role     string  `json:"role"`
			GroupIDs []int64 `json:"group_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Dados de usuário inválidos", http.StatusBadRequest)
			return
		}

		req.Username = strings.TrimSpace(req.Username)
		req.FullName = strings.TrimSpace(req.FullName)
		if len(req.Username) < 3 || len(req.Username) > 64 {
			http.Error(w, "O nome de usuário deve possuir entre 3 e 64 caracteres", http.StatusBadRequest)
			return
		}
		if len(req.Password) < 8 {
			http.Error(w, "A senha deve possuir no mínimo 8 caracteres para segurança corporativa", http.StatusBadRequest)
			return
		}
		if req.Role != "admin" && req.Role != "operator" {
			req.Role = "operator"
		}

		user, err := s.storage.CreateUser(req.Username, req.Password, req.FullName, req.Role, req.GroupIDs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		jsonResponse(w, http.StatusCreated, user)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// PUT /api/v1/users/{id} & DELETE /api/v1/users/{id} (Admin Only)
func (s *Server) handleUserItem(w http.ResponseWriter, r *http.Request) {
	admin := s.requireAdmin(w, r)
	if admin == nil {
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/users/")
	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "ID de usuário inválido", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodPut:
		var req struct {
			Username string  `json:"username"`
			Password string  `json:"password"`
			FullName string  `json:"full_name"`
			Role     string  `json:"role"`
			GroupIDs []int64 `json:"group_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Dados inválidos", http.StatusBadRequest)
			return
		}

		req.Username = strings.TrimSpace(req.Username)
		req.FullName = strings.TrimSpace(req.FullName)
		if req.Username != "" && (len(req.Username) < 3 || len(req.Username) > 64) {
			http.Error(w, "O nome de usuário deve possuir entre 3 e 64 caracteres", http.StatusBadRequest)
			return
		}
		if req.Password != "" && len(req.Password) < 8 {
			http.Error(w, "A nova senha deve possuir no mínimo 8 caracteres para segurança corporativa", http.StatusBadRequest)
			return
		}

		if err := s.storage.UpdateUser(id, req.Username, req.Password, req.FullName, req.Role, req.GroupIDs); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		updated, _ := s.storage.GetUserByID(id)
		jsonResponse(w, http.StatusOK, updated)

	case http.MethodDelete:
		if id == admin.ID {
			http.Error(w, "Você não pode excluir sua própria conta de administrador em uso.", http.StatusBadRequest)
			return
		}
		if err := s.storage.DeleteUser(id); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		jsonResponse(w, http.StatusOK, map[string]string{"message": "Usuário excluído com sucesso"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
