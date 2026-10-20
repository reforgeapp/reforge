package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/budget"
	"github.com/reforgeapp/reforge/pkg/campaign"
)

func (s *Server) RegisterCampaigns(service *campaign.Service) {
	group := s.Router.Group("/api/v1/orgs/:orgID", s.IdentitySession())
	group.POST("/campaign-previews", func(c *gin.Context) {
		var in campaign.Input
		if !identityJSON(c, &in) {
			return
		}
		session, _ := SessionFromContext(c)
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(3 * time.Minute))
		out, err := service.Preview(c.Request.Context(), session, c.Param("orgID"), in)
		if err != nil {
			campaignFailure(c, err)
			return
		}
		c.JSON(http.StatusOK, out)
	})
	group.POST("/campaigns", func(c *gin.Context) {
		var in struct {
			PreviewID      string `json:"preview_id"`
			IdempotencyKey string `json:"idempotency_key"`
		}
		if !identityJSON(c, &in) {
			return
		}
		session, _ := SessionFromContext(c)
		out, err := service.Create(c.Request.Context(), session, c.Param("orgID"), in.PreviewID, in.IdempotencyKey, c.GetString("request_id"))
		if err != nil {
			campaignFailure(c, err)
			return
		}
		campaignResponse(c, http.StatusCreated, out)
	})
	group.GET("/campaigns", func(c *gin.Context) {
		limit, cursor, ok := identityPage(c)
		if !ok {
			return
		}
		if limit > 100 {
			Fail(c, http.StatusBadRequest, "invalid_request", "Campaign page size must be between 1 and 100", false)
			return
		}
		session, _ := SessionFromContext(c)
		out, err := service.List(c.Request.Context(), session, c.Param("orgID"), c.Query("state"), cursor, limit)
		if err != nil {
			campaignFailure(c, err)
			return
		}
		c.JSON(http.StatusOK, out)
	})
	group.GET("/campaigns/:campaignID", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		out, err := service.Get(c.Request.Context(), session, c.Param("orgID"), c.Param("campaignID"))
		if err != nil {
			campaignFailure(c, err)
			return
		}
		campaignResponse(c, http.StatusOK, out)
	})
	group.GET("/campaigns/:campaignID/members", func(c *gin.Context) {
		limit, cursor, ok := identityPage(c)
		if !ok {
			return
		}
		if limit > 100 {
			Fail(c, http.StatusBadRequest, "invalid_request", "Campaign member page size must be between 1 and 100", false)
			return
		}
		session, _ := SessionFromContext(c)
		out, err := service.Members(c.Request.Context(), session, c.Param("orgID"), c.Param("campaignID"), cursor, limit)
		if err != nil {
			campaignFailure(c, err)
			return
		}
		c.JSON(http.StatusOK, out)
	})
	for _, action := range []string{"start", "pause", "resume", "cancel"} {
		group.POST("/campaigns/:campaignID/"+action, func(c *gin.Context) {
			version, ok := identityVersion(c)
			if !ok {
				return
			}
			var in struct {
				Reason        string `json:"reason"`
				StageDecision string `json:"stage_decision,omitempty"`
			}
			if !identityJSON(c, &in) {
				return
			}
			session, _ := SessionFromContext(c)
			out, err := service.Control(c.Request.Context(), session, c.Param("orgID"), c.Param("campaignID"), action, in.Reason, in.StageDecision, version, c.GetString("request_id"))
			if err != nil {
				campaignFailure(c, err)
				return
			}
			campaignResponse(c, http.StatusOK, out)
		})
	}
}

func campaignResponse(c *gin.Context, status int, value campaign.Campaign) {
	c.Header("ETag", strconv.Quote(strconv.FormatInt(value.Version, 10)))
	c.JSON(status, value)
}

func campaignFailure(c *gin.Context, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		Fail(c, http.StatusNotFound, "campaign_not_found", "Campaign record not found", false)
	case errors.Is(err, budget.ErrCapacity), errors.Is(err, budget.ErrUnknown), errors.Is(err, budget.ErrConflict), errors.Is(err, budget.ErrRevoked), errors.Is(err, budget.ErrInvalid):
		budgetFailure(c, err)
	case errors.Is(err, auth.ErrConflict):
		Fail(c, http.StatusConflict, "campaign_conflict", "Campaign evidence, version or stage changed; refresh and review before retrying", false)
	default:
		IdentityFailure(c, err)
	}
}
