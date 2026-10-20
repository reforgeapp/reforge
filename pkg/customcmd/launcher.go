package customcmd

import (
	"context"

	"github.com/reforgeapp/reforge/pkg/sandbox"
)

type SandboxLauncher struct {
	Runtime sandbox.SandboxRuntime
}

func (l SandboxLauncher) Launch(ctx context.Context, spec Spec) (sandbox.CommandResult, error) {
	if l.Runtime == nil {
		return sandbox.CommandResult{}, ErrInvalid
	}
	args := append([]string{spec.Executable}, spec.Args...)
	return l.Runtime.ExecuteBoundedCommand(ctx, spec.Workspace, sandbox.Command{
		Args:           args,
		Directory:      spec.Directory,
		Timeout:        spec.Timeout,
		MaxOutputBytes: spec.MaxOutput,
		NetworkProfile: "none",
		Stdin:          spec.Stdin,
	})
}
