package providers

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/connections"
	"reforge/internal/domain"
	"reforge/internal/network"
	"reforge/internal/privateconnector"
	"reforge/internal/runner"
)

func RegisterPrivate(service *connections.Service, connector *privateconnector.Connector, runners *runner.Service) {
	service.RegisterPrivateProbe(func(ctx context.Context, c connections.Connection, authorize connections.ProbeAuthorization) error {
		if c.Route == nil || c.Route.RevokedAt != nil {
			return connections.ErrRunnerRequired
		}
		kind := privateconnector.ForgeProbe
		if c.Kind == "model" {
			kind = privateconnector.ModelProbe
		} else if c.Kind != "forge" && c.Kind != "delivery" {
			return authorize(ctx, func(context.Context, pgx.Tx, connections.Resolved) (connections.ProbeResult, error) {
				return connections.ProbeResult{State: "disabled", Reason: "Official runtime qualification and isolated account custody are required"}, nil
			})
		}
		op := privateconnector.Operation{ID: domain.NewID(), Kind: kind}
		_, err := connector.Dispatch(ctx, privateconnector.Target{OrgID: c.OrgID, RunnerID: c.Route.RunnerID}, op, func(ctx context.Context, ready privateconnector.Ready, deliver privateconnector.Deliver) error {
			return authorize(ctx, func(ctx context.Context, tx pgx.Tx, r connections.Resolved) (connections.ProbeResult, error) {
				if r.Connection.ID != c.ID || r.Connection.OrgID != ready.OrgID || r.Connection.Route == nil || r.Connection.Route.RunnerID != ready.ID {
					return connections.ProbeResult{}, auth.ErrForbidden
				}
				if err := runners.ValidatePrivateSupervisorTx(ctx, tx, ready.Runner, ready.CredentialHash); err != nil {
					return connections.ProbeResult{}, err
				}
				result, err := deliver(privateconnector.GrantSpec{OperationID: op.ID, AuthorityID: op.ID, RunnerVersion: ready.Version, CredentialHash: ready.CredentialHash, Connection: PrivateConnection(r)})
				if err != nil {
					return connections.ProbeResult{}, err
				}
				if kind == privateconnector.ModelProbe {
					if result.ModelCapabilities == nil || result.ModelCapabilities.Provider != c.Provider {
						return connections.ProbeResult{}, privateconnector.ErrInvalid
					}
					return connections.ProbeResult{State: "healthy", Reason: "Private endpoint metadata returned; inference capabilities and usage limits need a qualified model turn", Capabilities: result.ModelCapabilities.Features}, nil
				}
				if result.Capabilities == nil || result.Capabilities.Provider != c.Provider {
					return connections.ProbeResult{}, privateconnector.ErrInvalid
				}
				return connections.ProbeResult{State: "healthy", Reason: "Authenticated through the enrolled private runner; write capabilities require separate qualification", Capabilities: result.Capabilities.Features, ServerVersion: result.Capabilities.ServerVersion}, nil
			})
		})
		if errors.Is(err, privateconnector.ErrUnsupported) {
			return connections.ErrRunnerRequired
		}
		return err
	})
}
func PrivateConnection(r connections.Resolved) privateconnector.Connection {
	c := r.Connection
	out := privateconnector.Connection{CheckPublishers: r.CheckPublishers, Kind: c.Kind, AuthKind: c.Settings.AuthKind, AppID: c.Settings.AppID, InstallationID: c.Settings.InstallationID, Model: c.Settings.Model, Profile: c.Settings.Profile, OrgID: c.OrgID, ID: c.ID, Version: c.Version, CredentialVersion: c.CredentialVersion, Provider: c.Provider, Endpoint: c.Endpoint, CAPEM: []byte(c.Settings.CAPEM), Secret: r.Secret, Route: network.PrivateRoute{OrgID: c.OrgID, ConnectionID: c.ID, RunnerID: c.Route.RunnerID, Host: c.Route.Host, CIDRs: append([]string(nil), c.Route.CIDRs...)}}
	if r.Protection != nil {
		p := r.Protection
		out.Protection = &privateconnector.ProtectionCredential{ID: p.Connection.ID, Version: p.Connection.Version, CredentialVersion: p.Connection.CredentialVersion, Secret: p.Secret}
	}
	return out
}
