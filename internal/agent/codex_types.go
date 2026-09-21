package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"reforge/internal/auth"
	"time"

	"reforge/internal/domain"
	"reforge/internal/sandbox"
)

const CodexVersion = "0.150.1"
const CodexBillingRoute = "codex.chatgpt_subscription"

var ErrDisabled = errors.New("official agent route is not qualified")
var ErrDenied = errors.New("official agent effect denied")
var ErrProtocol = errors.New("official agent protocol violation")
var ErrUncertain = errors.New("official agent outcome uncertain; reconcile before retry")

type Binding struct {
	OrgID             string `json:"org_id"`
	ConnectionID      string `json:"connection_id"`
	RunnerID          string `json:"runner_id,omitempty"`
	ConnectionVersion int64  `json:"connection_version"`
	CredentialVersion int64  `json:"credential_version"`
	AccountID         string `json:"account_id"`
	Model             string `json:"model"`
	RuntimeDigest     string `json:"runtime_digest"`
	Deployment        string `json:"deployment"`
}
type Qualification struct {
	Binding               Binding   `json:"binding"`
	EvidenceID            string    `json:"evidence_id"`
	CheckedAt             time.Time `json:"checked_at"`
	ExpiresAt             time.Time `json:"expires_at"`
	AuthCustody           bool      `json:"auth_custody"`
	NativeToolContainment bool      `json:"native_tool_containment"`
	Terms                 bool      `json:"terms"`
	Topology              bool      `json:"topology"`
	Entitlement           bool      `json:"entitlement"`
	Quota                 bool      `json:"quota"`
	NoPaidOverage         bool      `json:"no_paid_overage"`
}
type Transport interface {
	io.Reader
	io.Writer
	io.Closer
}
type Runtime struct {
	Transport Transport
	Binding   Binding
}
type Effect struct {
	Binding     Binding
	Request     Request
	OperationID string
	Kind        string
	CallID      string
	Arguments   json.RawMessage
}
type UserAction struct {
	ID     string
	UserID string
	Kind   string
}
type CodexConfig struct {
	Binding             Binding
	Open                func(context.Context, Binding) (Runtime, error)
	Qualify             func(context.Context, Binding) (Qualification, error)
	Authorize           func(context.Context, Effect, func() error) error
	AuthorizeUserAction func(context.Context, Binding, UserAction, func() error) error
	Sandbox             sandbox.SandboxRuntime
	Timeout             time.Duration
	MaxToolCalls        int
}
type Login struct {
	ID   string `json:"id"`
	URL  string `json:"url"`
	Code string `json:"code,omitempty"`
}

func (l Login) String() string       { return "official login [redacted]" }
func (l Login) GoString() string     { return l.String() }
func (l Login) LogValue() slog.Value { return slog.StringValue(l.String()) }
func disabledFeature(reason, action string) domain.Capability {
	return domain.Capability{State: domain.Unsupported, Scope: "configured official runtime", Reason: reason + "; " + action, Source: "T16 deployment qualification", LastChecked: time.Now().UTC()}
}
func qualified(q Qualification, b Binding, custodyOnly bool) bool {
	if q.Binding != b || !auth.ValidID(q.EvidenceID) || q.CheckedAt.IsZero() || q.CheckedAt.After(time.Now()) || !q.ExpiresAt.After(time.Now()) || !q.AuthCustody || !q.Topology {
		return false
	}
	return custodyOnly || q.NativeToolContainment && q.Terms && q.Entitlement && q.Quota && q.NoPaidOverage
}
func OfficialSupportMatrix() map[string]domain.Capability {
	return map[string]domain.Capability{
		"codex":  disabledFeature("managed app-server bridge implemented; no deployment certified", "qualify pinned runtime, account custody, native tool containment, entitlement, quota, terms and topology"),
		"claude": disabledFeature("official binary hosting and custom subscription SDK integrations have different permission requirements", "retain unmodified binary and native authentication; confirm deployment terms and certify pre-effect isolation before adding a bridge"),
		"gemini": disabledFeature("headless authentication does not qualify unattended hosted subscription use", "confirm customer plan and deployment permission; certify official binary isolation without extracting CLI OAuth credentials"),
	}
}

func qualificationFeatures(q Qualification, b Binding) map[string]domain.Capability {
	valid := q.Binding == b && auth.ValidID(q.EvidenceID) && !q.CheckedAt.IsZero() && !q.CheckedAt.After(time.Now()) && q.ExpiresAt.After(time.Now())
	result := map[string]domain.Capability{}
	for _, item := range []struct {
		key    string
		pass   bool
		action string
	}{
		{"auth_custody", q.AuthCustody, "verify official credential custody and immutable account binding on an isolated customer runtime"},
		{"native_tool_containment", q.NativeToolContainment, "prove implicit native reads cannot access credentials, other workspaces or host processes"},
		{"terms", q.Terms, "record deployment-specific provider permission and dated sources"},
		{"topology", q.Topology, "qualify the exact worker isolation, account custody and provider networking"},
		{"entitlement", q.Entitlement, "verify this customer account and pinned model are entitled to the selected official route"},
		{"quota", q.Quota, "verify runtime quota and concurrency limits for internal model requests"},
		{"no_paid_overage", q.NoPaidOverage, "verify paid credits, API fallback and automatic overage cannot activate"},
	} {
		state := domain.Unknown
		reason := item.action
		if valid && item.pass {
			state = domain.Supported
			reason = "verified by dated deployment evidence"
		}
		result[item.key] = domain.Capability{State: state, Scope: "exact runtime, model, account and deployment binding", Reason: reason, Source: q.EvidenceID, LastChecked: q.CheckedAt}
	}
	return result
}
