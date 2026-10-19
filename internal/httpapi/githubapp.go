package httpapi

import (
	"errors"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/githubapp"
	"github.com/reforgeapp/reforge/internal/inventory"
)

var githubHandoffPage = template.Must(template.New("handoff").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Continue to GitHub</title>{{if .Stylesheet}}<link rel="stylesheet" href="{{.Stylesheet}}">{{end}}</head><body><header class="topbar"><span class="brand"><span class="brand-mark" aria-hidden="true">R</span>Reforge</span></header><main class="content"><div class="page-header"><div><h1>Create GitHub App</h1><p>Continue to GitHub to create and install the App for this organisation.</p></div></div><form method="post" action="{{.Action}}"><input type="hidden" name="manifest" value="{{.Manifest}}"><button class="button button-primary" type="submit">Continue to GitHub</button></form></main></body></html>`))

var stylesheetPattern = regexp.MustCompile(`<link[^>]+rel="stylesheet"[^>]+href="([^"]+\.css)"`)

type handoffPage struct {
	githubapp.Handoff
	Stylesheet string
}

func (s *Server) handoffStylesheet() string {
	body, err := os.ReadFile(filepath.Join(s.Config.WebDir, "index.html"))
	if err != nil {
		return ""
	}
	if m := stylesheetPattern.FindSubmatch(body); m != nil {
		return string(m[1])
	}
	return ""
}

func (s *Server) RegisterGitHubApp(service *githubapp.Service) {
	g := s.Router.Group("/api/v1/orgs/:orgID/github-app", s.IdentitySession())
	g.GET("", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		status, err := service.Status(c.Request.Context(), session, c.Param("orgID"))
		if err != nil {
			githubAppFailure(c, err)
			return
		}
		c.JSON(200, status)
	})
	g.POST("/setups", func(c *gin.Context) {
		var input struct {
			Name      string `json:"name"`
			GitHubOrg string `json:"github_org"`
		}
		if !identityJSON(c, &input) {
			return
		}
		session, _ := SessionFromContext(c)
		created, err := service.Start(c.Request.Context(), session, c.Param("orgID"), input.Name, input.GitHubOrg, c.GetString("request_id"))
		if err != nil {
			githubAppFailure(c, err)
			return
		}
		c.JSON(201, created)
	})
	g.DELETE("/setups/:setupID", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		if err := service.Cancel(c.Request.Context(), session, c.Param("orgID"), c.Param("setupID"), c.GetString("request_id")); err != nil {
			githubAppFailure(c, err)
			return
		}
		c.Status(204)
	})
	s.Router.GET("/auth/github/setup/:setupID", func(c *gin.Context) {
		session, ok := s.callbackSession(c)
		if !ok {
			return
		}
		handoff, err := service.Handoff(c.Request.Context(), session, c.Param("setupID"))
		if err != nil {
			githubRedirect(c, session, githubapp.Result{OrgID: handoff.OrgID, Outcome: "expired"})
			return
		}
		if handoff.Location != "" {
			c.Redirect(303, handoff.Location)
			return
		}
		c.Header("Content-Security-Policy", strings.Replace(c.Writer.Header().Get("Content-Security-Policy"), "form-action 'self'", "form-action "+service.WebOrigin(), 1))
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.Status(200)
		_ = githubHandoffPage.Execute(c.Writer, handoffPage{Handoff: handoff, Stylesheet: s.handoffStylesheet()})
	})
	s.Router.GET("/auth/github/manifest/callback", func(c *gin.Context) {
		session, ok := s.callbackSession(c)
		if !ok {
			return
		}
		githubRedirect(c, session, service.ManifestCallback(c.Request.Context(), session, c.Query("code"), c.Query("state")))
	})
	s.Router.GET("/auth/github/install/callback", func(c *gin.Context) {
		session, ok := s.callbackSession(c)
		if !ok {
			return
		}
		githubRedirect(c, session, service.InstallCallback(c.Request.Context(), session, c.Query("installation_id"), c.Query("setup_action"), c.Query("state")))
	})
	s.Router.GET("/auth/github/oauth/callback", func(c *gin.Context) {
		session, ok := s.callbackSession(c)
		if !ok {
			return
		}
		githubRedirect(c, session, service.OAuthCallback(c.Request.Context(), session, c.Query("code"), c.Query("state"), c.Query("error")))
	})
	s.Router.POST("/hooks/github/app", func(c *gin.Context) {
		body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, inventory.MaxWebhookBytes))
		if err != nil {
			Fail(c, 413, "payload_too_large", "Webhook body exceeds limit", false)
			return
		}
		if err = service.HandleWebhook(c.Request.Context(), c.Request.Header, body); err != nil {
			githubAppFailure(c, err)
			return
		}
		c.Status(202)
	})
}

func (s *Server) callbackSession(c *gin.Context) (auth.Session, bool) {
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("Cache-Control", "no-store")
	session, err := s.Auth.Authenticate(c.Request.Context(), s.Auth.RequestToken(c.Request))
	if err != nil || s.Auth.CheckRequest(&http.Request{Method: "GET", Host: c.Request.Host}, "") != nil {
		c.Redirect(303, "/?github_result=expired&github_reason=sign_in_required")
		c.Abort()
		return session, false
	}
	return session, true
}

func githubRedirect(c *gin.Context, session auth.Session, r githubapp.Result) {
	if r.Outcome == "redirect" {
		c.Redirect(303, r.Location)
		return
	}
	org := r.OrgID
	if org == "" && len(session.Organisations) == 1 {
		org = session.Organisations[0].ID
	}
	if !auth.ValidID(org) {
		c.Redirect(303, "/?github_result="+url.QueryEscape(r.Outcome))
		return
	}
	query := url.Values{"github_result": {r.Outcome}}
	if r.Reason != "" {
		query.Set("github_reason", r.Reason)
	}
	if r.Outcome == "connected" {
		query.Set("connection", r.ConnectionID)
	}
	c.Redirect(303, "/org/"+org+"/connections?"+query.Encode())
}

func githubAppFailure(c *gin.Context, err error) {
	switch {
	case errors.Is(err, githubapp.ErrUnavailable):
		Fail(c, 409, "github_app_unavailable", "Guided GitHub App setup is unavailable; use a personal access token or manual App, or ask the operator to configure the GitHub App", false)
	default:
		IdentityFailure(c, err)
	}
}
