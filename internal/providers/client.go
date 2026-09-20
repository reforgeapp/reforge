package providers

import (
	"context"

	"github.com/jackc/pgx/v5"
	"reforge/internal/connections"
	"reforge/internal/privateconnector"
	"reforge/internal/source"
)

type Client interface {
	Read(context.Context, string, string, privateconnector.Operation, func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error)
	Write(context.Context, string, string, privateconnector.Operation, func(context.Context, pgx.Tx, connections.Connection) (string, error), func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error)
	SourceReader(string, string, func(context.Context, pgx.Tx, connections.Connection) error) source.Reader
	ForProtection(string, map[string]string) Client
}
