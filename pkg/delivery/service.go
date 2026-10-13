package delivery

import (
	"github.com/reforgeapp/reforge/pkg/deployment"
	"github.com/reforgeapp/reforge/pkg/gitops"
	"github.com/reforgeapp/reforge/pkg/providers"
	"github.com/reforgeapp/reforge/pkg/release"
)

type Service struct {
	Releases  *release.Service
	Native    *deployment.Service
	GitOps    *gitops.Service
	Providers providers.Client
}

type Result struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	State string `json:"state"`
}
