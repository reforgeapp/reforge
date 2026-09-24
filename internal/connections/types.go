package connections

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"log/slog"
	"net/http"
	"reforge/internal/domain"
	"time"
)

type Settings struct {
	Namespace      string   `json:"namespace,omitempty"`
	Model          string   `json:"model,omitempty"`
	Profile        string   `json:"profile,omitempty"`
	AuthKind       string   `json:"auth_kind"`
	BillingRoute   string   `json:"billing_route"`
	AppID          string   `json:"app_id,omitempty"`
	InstallationID string   `json:"installation_id,omitempty"`
	RuntimeVersion string   `json:"runtime_version,omitempty"`
	CAPEM          string   `json:"ca_pem,omitempty"`
	AllowedModels  []string `json:"allowed_models,omitempty"`
}

type Route struct {
	RunnerID   string     `json:"runner_id"`
	Host       string     `json:"host"`
	CIDRs      []string   `json:"cidrs"`
	ApprovedAt time.Time  `json:"approved_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

type Connection struct {
	ID                string                       `json:"id"`
	OrgID             string                       `json:"org_id"`
	Kind              string                       `json:"kind"`
	Provider          string                       `json:"provider"`
	Name              string                       `json:"name"`
	Endpoint          string                       `json:"endpoint"`
	Settings          Settings                     `json:"settings"`
	State             string                       `json:"state"`
	Reason            string                       `json:"reason"`
	Capabilities      map[string]domain.Capability `json:"capabilities"`
	ServerVersion     string                       `json:"server_version"`
	VerifiedAt        *time.Time                   `json:"verified_at"`
	CredentialVersion int64                        `json:"credential_version"`
	Version           int64                        `json:"version"`
	Route             *Route                       `json:"private_route,omitempty"`
	SecretID          *string                      `json:"-"`
}

type CreateRequest struct {
	Kind         string   `json:"kind"`
	Provider     string   `json:"provider"`
	Name         string   `json:"name"`
	Endpoint     string   `json:"endpoint"`
	Settings     Settings `json:"settings"`
	Secret       string   `json:"secret"`
	PrivateRoute *Route   `json:"private_route,omitempty"`
}

func (r CreateRequest) LogValue() slog.Value {
	return slog.GroupValue(slog.String("kind", r.Kind), slog.String("provider", r.Provider))
}
func (r CreateRequest) String() string   { return "connection setup [credential redacted]" }
func (r CreateRequest) GoString() string { return r.String() }
func (r CreateRequest) MarshalJSON() ([]byte, error) {
	type safe CreateRequest
	v := safe(r)
	v.Secret = ""
	return json.Marshal(struct {
		safe
		Secret any `json:"secret,omitempty"`
	}{safe: v})
}

type CatalogSettings struct {
	Model        string `json:"model,omitempty"`
	Profile      string `json:"profile,omitempty"`
	AuthKind     string `json:"auth_kind"`
	BillingRoute string `json:"billing_route"`
	CAPEM        string `json:"ca_pem,omitempty"`
}

type CatalogRequest struct {
	Provider string          `json:"provider"`
	Endpoint string          `json:"endpoint"`
	Secret   string          `json:"secret"`
	Settings CatalogSettings `json:"settings"`
}

func (r CatalogRequest) LogValue() slog.Value {
	return slog.GroupValue(slog.String("provider", r.Provider))
}
func (r CatalogRequest) String() string   { return "model catalog request [credential redacted]" }
func (r CatalogRequest) GoString() string { return r.String() }
func (r CatalogRequest) MarshalJSON() ([]byte, error) {
	type safe CatalogRequest
	v := safe(r)
	v.Secret = ""
	return json.Marshal(v)
}

type CatalogItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Disabled bool   `json:"disabled,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type Cataloger func(context.Context, Resolved) ([]CatalogItem, error)

type Resolved struct {
	Protection      *Resolved         `json:"-"`
	CheckPublishers map[string]string `json:"-"`
	Connection      Connection
	Secret          string       `json:"-"`
	Client          *http.Client `json:"-"`
}

func (r Resolved) String() string   { return "resolved connection " + r.Connection.ID }
func (r Resolved) GoString() string { return r.String() }
func (r Resolved) LogValue() slog.Value {
	return slog.GroupValue(slog.String("connection_id", r.Connection.ID))
}

type ProbeResult struct {
	Capabilities  map[string]domain.Capability `json:"capabilities"`
	ServerVersion string                       `json:"server_version"`
	State         string                       `json:"state"`
	Reason        string                       `json:"reason"`
}
type Prober func(context.Context, Resolved) (ProbeResult, error)

type ProbeCall func(context.Context, pgx.Tx, Resolved) (ProbeResult, error)
type ProbeAuthorization func(context.Context, ProbeCall) error
type PrivateProber func(context.Context, Connection, ProbeAuthorization) error
