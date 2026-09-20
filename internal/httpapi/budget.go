package httpapi

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"reforge/internal/budget"
)

func (s *Server) RegisterBudget(service *budget.Service) {
	g := s.Router.Group("/api/v1/orgs/:orgID", s.IdentitySession())
	g.GET("/budgets/:scopeKind/:scopeID", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		value, err := service.GetLimit(c.Request.Context(), session, c.Param("orgID"), budget.Scope{Kind: c.Param("scopeKind"), ID: c.Param("scopeID")})
		if err != nil {
			budgetFailure(c, err)
			return
		}
		budgetResponse(c, value.Version, value)
	})
	g.PUT("/budgets/:scopeKind/:scopeID", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		var value budget.Limit
		if !identityJSON(c, &value) {
			return
		}
		value.Scope = budget.Scope{Kind: c.Param("scopeKind"), ID: c.Param("scopeID")}
		session, _ := SessionFromContext(c)
		value, err := service.PutLimit(c.Request.Context(), session, c.Param("orgID"), value, version, c.GetString("request_id"))
		if err != nil {
			budgetFailure(c, err)
			return
		}
		budgetResponse(c, value.Version, value)
	})
	g.GET("/budget-routes/:connectionID", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		value, err := service.GetRoute(c.Request.Context(), session, c.Param("orgID"), c.Param("connectionID"), c.Query("model"), c.DefaultQuery("route", "default"))
		if err != nil {
			budgetFailure(c, err)
			return
		}
		budgetResponse(c, value.Version, value)
	})
	g.PUT("/budget-routes/:connectionID", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		var value budget.Route
		if !identityJSON(c, &value) {
			return
		}
		value.ConnectionID = c.Param("connectionID")
		session, _ := SessionFromContext(c)
		value, err := service.PutRoute(c.Request.Context(), session, c.Param("orgID"), value, version, c.GetString("request_id"))
		if err != nil {
			budgetFailure(c, err)
			return
		}
		budgetResponse(c, value.Version, value)
	})
	g.GET("/usage/:reservationID", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		value, err := service.Get(c.Request.Context(), session, c.Param("orgID"), c.Param("reservationID"))
		if err != nil {
			budgetFailure(c, err)
			return
		}
		c.JSON(200, value)
	})
}

func budgetResponse(c *gin.Context, version int64, value any) {
	c.Header("ETag", strconv.Quote(strconv.FormatInt(version, 10)))
	c.JSON(200, value)
}

func budgetFailure(c *gin.Context, err error) {
	switch {
	case errors.Is(err, budget.ErrCapacity):
		Fail(c, 409, "budget_exhausted", "Increase or wait for the applicable budget; outstanding unknown usage remains reserved", false)
	case errors.Is(err, budget.ErrUnknown):
		Fail(c, 409, "budget_unconfigured", "Configure the budget, pricing and qualified billing route before execution", false)
	case errors.Is(err, budget.ErrConflict):
		Fail(c, 409, "budget_conflict", "Budget or billing route changed; reload its current version", false)
	case errors.Is(err, budget.ErrRevoked):
		Fail(c, 409, "budget_revoked", "Execution authority changed; review the task before retrying", false)
	case errors.Is(err, budget.ErrInvalid):
		Fail(c, 400, "invalid_budget", "Budget or billing route configuration is invalid", false)
	default:
		workflowFailure(c, err)
	}
}
