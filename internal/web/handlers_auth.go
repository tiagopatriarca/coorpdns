package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coorpdns/coorpdns/internal/storage"
)

func (s *Server) getAuthUser(r *http.Request) *storage.UserWithGroups {
	// 1. Tenta obter do Cookie seguro
	var token string
	if cookie, err := r.Cookie("coorpdns_session"); err == nil && cookie != nil && cookie.Value != "" {
		token = cookie.Value
	}
	// 2. Tenta obter do cabeçalho Authorization: Bearer <token>
	if token == "" {
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		}
	}
	// 3. Tenta obter da query string (?session=...)
	if token == "" {
		token = r.URL.Query().Get("session")
	}

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
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clientIP := getRealClientIP(r)
	if s.loginLimiter != nil && !s.loginLimiter.Allow(clientIP) {
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

	user, err := s.storage.AuthenticateUser(req.Username, req.Password)
	if err != nil {
		if s.loginLimiter != nil {
			s.loginLimiter.RecordFailedAttempt(clientIP)
		}
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	if s.loginLimiter != nil {
		s.loginLimiter.Reset(clientIP)
	}

	token, err := s.storage.CreateSession(user.ID, 30*time.Minute)
	if err != nil {
		http.Error(w, "Erro ao criar sessão: "+err.Error(), http.StatusInternalServerError)
		return
	}

	isSecure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	// Cookie de Sessão: MaxAge e Expires não são definidos para que o navegador
	// destrua o cookie da memória ao fechar o navegador/janela
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
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Remove cookie
	isSecure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     "coorpdns_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	// Deleta do banco
	token := ""
	if cookie, err := r.Cookie("coorpdns_session"); err == nil && cookie != nil && cookie.Value != "" {
		token = cookie.Value
	}
	if token == "" {
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		}
	}
	if token != "" {
		s.storage.DeleteSession(token)
	}

	jsonResponse(w, http.StatusOK, map[string]string{
		"message": "Sessão finalizada com sucesso",
	})
}

// GET /api/v1/auth/me
func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	u := s.getAuthUser(r)
	if u == nil {
		http.Error(w, "Não autenticado", http.StatusUnauthorized)
		return
	}
	jsonResponse(w, http.StatusOK, u)
}

// POST /api/v1/auth/change-password
func (s *Server) handleAuthChangePassword(w http.ResponseWriter, r *http.Request) {
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

	if err := s.storage.ChangePassword(u.ID, req.OldPassword, req.NewPassword); err != nil {
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
	case "GET":
		users, err := s.storage.ListUsers()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, http.StatusOK, users)

	case "POST":
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
		if len(req.Username) < 3 {
			http.Error(w, "O nome de usuário deve possuir no mínimo 3 caracteres", http.StatusBadRequest)
			return
		}
		if len(req.Password) < 6 {
			http.Error(w, "A senha deve possuir no mínimo 6 caracteres para segurança corporativa", http.StatusBadRequest)
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
	case "PUT":
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
		if req.Username != "" && len(req.Username) < 3 {
			http.Error(w, "O nome de usuário deve possuir no mínimo 3 caracteres", http.StatusBadRequest)
			return
		}
		if req.Password != "" && len(req.Password) < 6 {
			http.Error(w, "A nova senha deve possuir no mínimo 6 caracteres para segurança corporativa", http.StatusBadRequest)
			return
		}

		if err := s.storage.UpdateUser(id, req.Username, req.Password, req.FullName, req.Role, req.GroupIDs); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		updated, _ := s.storage.GetUserByID(id)
		jsonResponse(w, http.StatusOK, updated)

	case "DELETE":
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
