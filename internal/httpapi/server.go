package httpapi

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/config"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/heartbeat"
	"github.com/reforgeapp/reforge/internal/store"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Server struct {
	Config config.Config
	Store  *store.Store
	Router *gin.Engine
	Auth   *auth.Service
}

func New(cfg config.Config, db *store.Store) *Server {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.HandleMethodNotAllowed = true
	r.MaxMultipartMemory = 1 << 20
	s := &Server{Config: cfg, Store: db, Router: r}
	r.Use(s.boundaries())
	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	r.GET("/readyz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if db == nil || db.Pool.Ping(ctx) != nil {
			Fail(c, 503, "unavailable", "Database unavailable", true)
			return
		}
		c.JSON(200, gin.H{"status": "ready"})
	})
	r.GET("/statusz", func(c *gin.Context) {
		loops, healthy := heartbeat.Status()
		status := 200
		if !healthy {
			status = 503
		}
		c.JSON(status, gin.H{"healthy": healthy, "loops": loops})
	})
	r.GET("/api/v1/meta", func(c *gin.Context) {
		c.JSON(200, gin.H{"name": "Reforge", "version": "0.1.0-dev", "edition": cfg.Edition, "development": cfg.Development, "fixture_auth": cfg.FixtureAuth, "docs_url": cfg.DocsURL})
	})
	r.NoMethod(func(c *gin.Context) { Fail(c, 405, "method_not_allowed", "Method not allowed", false) })
	r.NoRoute(s.frontend)
	return s
}

func (s *Server) boundaries() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := domain.NewID()
		c.Set("request_id", id)
		c.Header("X-Request-ID", id)
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "same-origin")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		if strings.HasPrefix(c.Request.URL.Path, "/api/") || strings.HasPrefix(c.Request.URL.Path, "/auth/") {
			c.Header("Cache-Control", "no-store")
		}
		if !s.Config.Development {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		bodyLimit := int64(2 << 20)
		if c.Request.Method == http.MethodPost && c.Request.URL.Path == "/runner/v1/private/results" {
			bodyLimit = 6 << 20
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, bodyLimit)
		start := time.Now()
		defer func() {
			if recover() != nil {
				slog.Error("request panic", "request_id", id)
				if !c.Writer.Written() {
					Fail(c, 500, "internal", "Request failed", false)
				}
			}
			slog.Info("request", "request_id", id, "method", c.Request.Method, "route", c.FullPath(), "status", c.Writer.Status(), "duration_ms", time.Since(start).Milliseconds())
		}()
		c.Next()
	}
}

func Fail(c *gin.Context, status int, code, message string, retryable bool) {
	c.AbortWithStatusJSON(status, domain.Error{Code: code, Message: message, RequestID: c.GetString("request_id"), Retryable: retryable})
}

func (s *Server) frontend(c *gin.Context) {
	if c.Request.Method != "GET" && c.Request.Method != "HEAD" {
		Fail(c, 404, "not_found", "Not found", false)
		return
	}
	p := c.Request.URL.Path
	if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/runner/") || strings.HasPrefix(p, "/auth/") {
		Fail(c, 404, "not_found", "Not found", false)
		return
	}
	name := strings.TrimPrefix(p, "/")
	if name == "" {
		name = "index.html"
	}
	if !fs.ValidPath(name) || strings.Contains(name, "\\") {
		Fail(c, 404, "not_found", "Not found", false)
		return
	}
	if info, err := os.Stat(filepath.Join(s.Config.WebDir, name)); err == nil && !info.IsDir() {
		switch {
		case strings.HasPrefix(name, "assets/"):
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
		case name == "index.html":
			c.Header("Cache-Control", "no-store, must-revalidate")
		}
		c.File(filepath.Join(s.Config.WebDir, name))
		return
	}
	if strings.Contains(filepath.Base(name), ".") {
		Fail(c, 404, "not_found", "Not found", false)
		return
	}
	if _, err := os.Stat(filepath.Join(s.Config.WebDir, "index.html")); err != nil {
		Fail(c, 503, "frontend_unbuilt", "Run make build to build the interface", false)
		return
	}
	c.Header("Cache-Control", "no-store, must-revalidate")
	c.File(filepath.Join(s.Config.WebDir, "index.html"))
}
