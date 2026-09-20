package dbgen

import (
	"github.com/jackc/pgx/v5/pgtype"
)

type AuditEvent struct {
	ID           pgtype.UUID        `json:"id"`
	OrgID        pgtype.UUID        `json:"org_id"`
	RepositoryID pgtype.UUID        `json:"repository_id"`
	ActorID      string             `json:"actor_id"`
	Action       string             `json:"action"`
	ObjectID     string             `json:"object_id"`
	RequestID    string             `json:"request_id"`
	Data         []byte             `json:"data"`
	OccurredAt   pgtype.Timestamptz `json:"occurred_at"`
}

type Bootstrap struct {
	ID         bool               `json:"id"`
	TokenHash  string             `json:"token_hash"`
	ExpiresAt  pgtype.Timestamptz `json:"expires_at"`
	ConsumedAt pgtype.Timestamptz `json:"consumed_at"`
}

type Connection struct {
	OrgID             pgtype.UUID        `json:"org_id"`
	ID                pgtype.UUID        `json:"id"`
	Kind              string             `json:"kind"`
	Provider          string             `json:"provider"`
	Name              string             `json:"name"`
	Endpoint          string             `json:"endpoint"`
	Settings          []byte             `json:"settings"`
	State             string             `json:"state"`
	Reason            string             `json:"reason"`
	Capabilities      []byte             `json:"capabilities"`
	ServerVersion     string             `json:"server_version"`
	VerifiedAt        pgtype.Timestamptz `json:"verified_at"`
	SecretID          pgtype.UUID        `json:"secret_id"`
	CredentialVersion int64              `json:"credential_version"`
	Version           int64              `json:"version"`
	CreatedAt         pgtype.Timestamptz `json:"created_at"`
	RevokedAt         pgtype.Timestamptz `json:"revoked_at"`
}

type ConnectionRoute struct {
	OrgID        pgtype.UUID        `json:"org_id"`
	ConnectionID pgtype.UUID        `json:"connection_id"`
	RunnerID     pgtype.UUID        `json:"runner_id"`
	Hostname     string             `json:"hostname"`
	Cidrs        []string           `json:"cidrs"`
	ApprovedBy   pgtype.UUID        `json:"approved_by"`
	ApprovedAt   pgtype.Timestamptz `json:"approved_at"`
	RevokedAt    pgtype.Timestamptz `json:"revoked_at"`
}

type ConnectionSecret struct {
	OrgID        pgtype.UUID        `json:"org_id"`
	ID           pgtype.UUID        `json:"id"`
	ConnectionID pgtype.UUID        `json:"connection_id"`
	Version      int64              `json:"version"`
	Envelope     []byte             `json:"envelope"`
	RotatedAt    pgtype.Timestamptz `json:"rotated_at"`
}

type MemberRepository struct {
	OrgID        pgtype.UUID `json:"org_id"`
	UserID       pgtype.UUID `json:"user_id"`
	RepositoryID pgtype.UUID `json:"repository_id"`
}

type Membership struct {
	OrgID           pgtype.UUID `json:"org_id"`
	UserID          pgtype.UUID `json:"user_id"`
	Role            string      `json:"role"`
	AllRepositories bool        `json:"all_repositories"`
	Version         int64       `json:"version"`
}

type OidcLogin struct {
	StateHash   string             `json:"state_hash"`
	BrowserHash string             `json:"browser_hash"`
	Nonce       string             `json:"nonce"`
	Verifier    string             `json:"verifier"`
	ExpiresAt   pgtype.Timestamptz `json:"expires_at"`
}

type Organisation struct {
	ID        pgtype.UUID        `json:"id"`
	Name      string             `json:"name"`
	Version   int64              `json:"version"`
	Paused    bool               `json:"paused"`
	CreatedAt pgtype.Timestamptz `json:"created_at"`
}

type PolicyBinding struct {
	OrgID           pgtype.UUID `json:"org_id"`
	ScopeKind       string      `json:"scope_kind"`
	ScopeID         pgtype.UUID `json:"scope_id"`
	PolicyVersionID pgtype.UUID `json:"policy_version_id"`
	Version         int64       `json:"version"`
	PrimaryTeamID   pgtype.UUID `json:"primary_team_id"`
	SimulationHash  string      `json:"simulation_hash"`
}

type PolicyVersion struct {
	OrgID      pgtype.UUID        `json:"org_id"`
	ID         pgtype.UUID        `json:"id"`
	ScopeKind  string             `json:"scope_kind"`
	ScopeID    pgtype.UUID        `json:"scope_id"`
	Document   []byte             `json:"document"`
	PolicyHash string             `json:"policy_hash"`
	ActorID    pgtype.UUID        `json:"actor_id"`
	Reason     string             `json:"reason"`
	CreatedAt  pgtype.Timestamptz `json:"created_at"`
}

type Repository struct {
	OrgID         pgtype.UUID        `json:"org_id"`
	ID            pgtype.UUID        `json:"id"`
	ConnectionID  pgtype.UUID        `json:"connection_id"`
	NativeID      string             `json:"native_id"`
	Name          string             `json:"name"`
	Url           string             `json:"url"`
	DefaultBranch string             `json:"default_branch"`
	Provider      string             `json:"provider"`
	Archived      bool               `json:"archived"`
	Paused        bool               `json:"paused"`
	Accessible    bool               `json:"accessible"`
	LastSyncedAt  pgtype.Timestamptz `json:"last_synced_at"`
	Version       int64              `json:"version"`
}

type Session struct {
	ID        pgtype.UUID        `json:"id"`
	UserID    pgtype.UUID        `json:"user_id"`
	TokenHash string             `json:"token_hash"`
	CsrfToken string             `json:"csrf_token"`
	ExpiresAt pgtype.Timestamptz `json:"expires_at"`
	RevokedAt pgtype.Timestamptz `json:"revoked_at"`
	CreatedAt pgtype.Timestamptz `json:"created_at"`
}

type Team struct {
	OrgID   pgtype.UUID `json:"org_id"`
	ID      pgtype.UUID `json:"id"`
	Name    string      `json:"name"`
	Version int64       `json:"version"`
}

type TeamMembership struct {
	OrgID  pgtype.UUID `json:"org_id"`
	TeamID pgtype.UUID `json:"team_id"`
	UserID pgtype.UUID `json:"user_id"`
}

type TeamRepository struct {
	OrgID        pgtype.UUID `json:"org_id"`
	TeamID       pgtype.UUID `json:"team_id"`
	RepositoryID pgtype.UUID `json:"repository_id"`
}

type User struct {
	ID        pgtype.UUID        `json:"id"`
	Issuer    string             `json:"issuer"`
	Subject   string             `json:"subject"`
	Name      string             `json:"name"`
	Email     string             `json:"email"`
	CreatedAt pgtype.Timestamptz `json:"created_at"`
}
