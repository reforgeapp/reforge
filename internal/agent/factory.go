package agent

import (
	"context"
	"time"

	"github.com/reforgeapp/reforge/internal/sandbox"
)

type Launcher func(context.Context, Binding) (Runtime, error)

type Factory struct {
	launch       Launcher
	sandbox      sandbox.SandboxRuntime
	timeout      time.Duration
	maxToolCalls int
}

func NewFactory(launch Launcher, box sandbox.SandboxRuntime) *Factory {
	return &Factory{launch: launch, sandbox: box, timeout: 10 * time.Minute, maxToolCalls: 32}
}

func (f *Factory) Configured() bool { return f != nil && f.launch != nil }

func (f *Factory) Bridge(
	binding Binding,
	qualify func(context.Context, Binding) (Qualification, error),
	authorize func(context.Context, Effect, func() error) error,
	authorizeUser func(context.Context, Binding, UserAction, func() error) error,
) (*Codex, error) {
	if !f.Configured() {
		return nil, ErrDisabled
	}
	return NewCodex(CodexConfig{
		Binding:             binding,
		Open:                f.launch,
		Qualify:             qualify,
		Authorize:           authorize,
		AuthorizeUserAction: authorizeUser,
		Sandbox:             f.sandbox,
		Timeout:             f.timeout,
		MaxToolCalls:        f.maxToolCalls,
	})
}
