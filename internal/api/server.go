package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/wqedggc/shoreos/internal/repository/mysql"
	"github.com/wqedggc/shoreos/web"
)

type Server struct {
	store  *mysql.Store
	static fs.FS
}

type ctxKey string

const userKey ctxKey = "user"

func NewServer(store *mysql.Store) (*Server, error) {
	static, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		return nil, err
	}
	return &Server{store: store, static: static}, nil
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", s.health)
	r.Get("/readyz", s.ready)

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/bootstrap", s.bootstrap)
		r.Post("/auth/login", s.login)

		r.Group(func(r chi.Router) {
			r.Use(s.auth)
			r.Post("/auth/logout", s.logout)
			r.Get("/me", s.me)
			r.Patch("/me", s.updateMe)
			r.Get("/fire/scenarios", s.getProfiles)
			r.Put("/fire/scenarios/sync", s.syncProfiles)
			r.Post("/fire/scenarios", s.syncProfiles)
			r.Get("/fire/asset-snapshots", s.emptyList)
			r.Post("/fire/asset-snapshots", s.echoCreated)
			r.Post("/fire/projection-runs", s.echoCreated)
			r.Get("/fire/projection-runs/{id}", s.notFound)
			r.Get("/fire/summary", s.getProfiles)
		})
	})

	fileServer := http.FileServer(http.FS(s.static))
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "接口不存在")
			return
		}
		fileServer.ServeHTTP(w, r)
	})
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, s.static, "index.html")
	})
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		fileServer.ServeHTTP(w, r)
	})
	return r
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeData(w, map[string]string{"status": "ok"})
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "DB_NOT_READY", "数据库不可用")
		return
	}
	writeData(w, map[string]string{"status": "ready"})
}

func (s *Server) bootstrap(w http.ResponseWriter, r *http.Request) {
	user, token, err := s.store.Bootstrap(r.Context())
	if err != nil {
		log.Printf("bootstrap: %v", err)
		writeError(w, http.StatusInternalServerError, "BOOTSTRAP_FAILED", "初始化用户失败")
		return
	}
	writeData(w, map[string]any{"token": token, "user": user})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求体不是合法 JSON")
		return
	}
	user, token, err := s.store.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "账号或密码错误")
		return
	}
	writeData(w, map[string]any{"token": token, "user": user})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)
	if token != "" {
		_ = s.store.Logout(r.Context(), token)
	}
	writeData(w, map[string]string{"status": "ok"})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeData(w, currentUser(r))
}

func (s *Server) updateMe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DisplayName string `json:"displayName"`
		Avatar      string `json:"avatar"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求体不是合法 JSON")
		return
	}
	user, err := s.store.UpdateUser(r.Context(), currentUser(r).ID, req.DisplayName, req.Avatar)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UPDATE_FAILED", "用户信息保存失败")
		return
	}
	writeData(w, user)
}

func (s *Server) getProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := s.store.Profiles(r.Context(), currentUser(r).ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUERY_FAILED", "FIRE 档案读取失败")
		return
	}
	writeData(w, map[string]any{"profiles": profiles})
}

func (s *Server) syncProfiles(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Profiles []map[string]any `json:"profiles"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求体不是合法 JSON")
		return
	}
	if err := s.store.SyncProfiles(r.Context(), currentUser(r).ID, req.Profiles); err != nil {
		log.Printf("sync profiles: %v", err)
		writeError(w, http.StatusInternalServerError, "SAVE_FAILED", "FIRE 档案保存失败")
		return
	}
	writeData(w, map[string]any{"profiles": req.Profiles})
}

func (s *Server) emptyList(w http.ResponseWriter, r *http.Request) {
	writeData(w, []any{})
}

func (s *Server) echoCreated(w http.ResponseWriter, r *http.Request) {
	var payload map[string]any
	_ = json.NewDecoder(r.Body).Decode(&payload)
	writeJSON(w, http.StatusCreated, map[string]any{"data": payload})
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "NOT_FOUND", "资源不存在")
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "请先登录")
			return
		}
		user, err := s.store.UserByToken(r.Context(), token)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "登录已过期")
				return
			}
			writeError(w, http.StatusInternalServerError, "AUTH_FAILED", "认证失败")
			return
		}
		ctx := context.WithValue(r.Context(), userKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func currentUser(r *http.Request) mysql.User {
	user, _ := r.Context().Value(userKey).(mysql.User)
	return user
}

func bearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
}

func writeData(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": message}})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
