package recipes

import "errors"

var ErrUnsupported = errors.New("repository requires an explicit supported validation recipe")

type Command struct {
	ID             string   `json:"id"`
	Args           []string `json:"args"`
	Directory      string   `json:"directory"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	ReportFormat   string   `json:"report_format"`
}
type Recipe struct {
	Name           string    `json:"name"`
	Version        string    `json:"version"`
	Commands       []Command `json:"commands"`
	ProtectedPaths []string  `json:"protected_paths"`
	ManifestPaths  []string  `json:"manifest_paths"`
	AllowedPaths   []string  `json:"allowed_paths,omitempty"`
	MinProof       string    `json:"min_proof,omitempty"`
	ReviewOnly     bool      `json:"review_only,omitempty"`
	MinimumTests   int       `json:"minimum_tests"`
	MaxFiles       int       `json:"max_files"`
	MaxPatchBytes  int       `json:"max_patch_bytes"`
	MaxTurns       int       `json:"max_turns"`
	TimeoutSeconds int       `json:"timeout_seconds"`
}
