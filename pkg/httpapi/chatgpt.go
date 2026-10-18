package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/chatgpt"
	"github.com/reforgeapp/reforge/pkg/connections"
)

func (s *Server) RegisterChatGPT(service *connections.Service) {
	group := s.Router.Group("/api/v1/orgs/:orgID/chatgpt", s.IdentitySession())
	group.GET("", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		status, err := service.ChatGPTStatus(c.Request.Context(), session, c.Param("orgID"))
		if err != nil {
			connectionFailure(c, err)
			return
		}
		c.JSON(http.StatusOK, status)
	})
	group.POST("/setups", func(c *gin.Context) {
		var input struct {
			Name         string `json:"name"`
			ConnectionID string `json:"connection_id"`
			Version      int64  `json:"version"`
		}
		if !identityJSON(c, &input) {
			return
		}
		session, _ := SessionFromContext(c)
		browser := chatgpt.Random()
		location, err := service.BeginChatGPT(c.Request.Context(), session, c.Param("orgID"), input.Name, input.ConnectionID, input.Version, browser, c.GetString("request_id"))
		if err != nil {
			connectionFailure(c, err)
			return
		}
		http.SetCookie(c.Writer, s.chatGPTCookie(browser, 600))
		c.JSON(http.StatusCreated, gin.H{"url": location})
	})
	group.GET("/setups/:setupID", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		setup, err := service.ChatGPTSetup(c.Request.Context(), session, c.Param("orgID"), c.Param("setupID"))
		if err != nil {
			connectionFailure(c, err)
			return
		}
		c.JSON(http.StatusOK, setup)
	})
	group.POST("/setups/:setupID", func(c *gin.Context) {
		var input struct {
			Model string `json:"model"`
		}
		if !identityJSON(c, &input) {
			return
		}
		session, _ := SessionFromContext(c)
		connection, err := service.SaveChatGPT(c.Request.Context(), session, c.Param("orgID"), c.Param("setupID"), input.Model, c.GetString("request_id"))
		if err != nil {
			connectionFailure(c, err)
			return
		}
		connectionResponse(c, http.StatusCreated, connection)
	})
	s.Router.GET("/auth/openai/callback", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("Referrer-Policy", "no-referrer")
		session, err := s.Auth.Authenticate(c.Request.Context(), s.Auth.RequestToken(c.Request))
		if err != nil || s.Auth.CheckRequest(&http.Request{Method: http.MethodGet, Host: c.Request.Host}, "") != nil {
			c.Redirect(http.StatusSeeOther, "/?chatgpt_result=sign_in_required")
			return
		}
		browser := ""
		if cookie, err := c.Request.Cookie(s.chatGPTCookie("", 0).Name); err == nil {
			browser = cookie.Value
		}
		http.SetCookie(c.Writer, s.chatGPTCookie("", -1))
		query := c.Request.URL.Query()
		if len(query["state"]) != 1 || len(query["code"]) > 1 || len(query["error"]) > 1 {
			c.Redirect(http.StatusSeeOther, "/?chatgpt_result=failed")
			return
		}
		org, id, err := service.CompleteChatGPT(c.Request.Context(), session, query.Get("state"), browser, query.Get("code"), query.Get("error"))
		location := "/"
		if auth.ValidID(org) {
			location = "/org/" + org + "/connections"
		}
		result := url.Values{"chatgpt_result": {"failed"}, "connection_tab": {"models"}}
		if errors.Is(err, chatgpt.ErrPermission) {
			result.Set("chatgpt_result", "permission_required")
		}
		if err == nil {
			result.Set("chatgpt_result", "authorized")
			result.Set("chatgpt_setup", id)
		}
		c.Redirect(http.StatusSeeOther, location+"?"+result.Encode())
	})
}

func (s *Server) chatGPTCookie(value string, age int) *http.Cookie {
	name := "reforge_chatgpt"
	secure := strings.HasPrefix(s.Config.PublicURL, "https://")
	if secure {
		name = "__Host-reforge_chatgpt"
	}
	return &http.Cookie{Name: name, Value: value, Path: "/", Secure: secure, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: age}
}
