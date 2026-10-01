package github

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"reforge/internal/forge"
)

func (p *Provider) ListIssues(ctx context.Context, reference forge.RepoRef) ([]forge.Issue, error) {
	segments, err := repositoryPath(reference)
	if err != nil {
		return nil, err
	}
	status, headers, body, err := p.request(ctx, http.MethodGet, append(segments, "issues"), url.Values{"state": {"open"}, "labels": {"bug"}, "per_page": {"50"}}, nil)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, responseError(status, headers)
	}
	var raw []struct {
		Number      int       `json:"number"`
		Title       string    `json:"title"`
		Body        string    `json:"body"`
		HTMLURL     string    `json:"html_url"`
		PullRequest *struct{} `json:"pull_request"`
		Labels      []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}
	if err := decode(body, &raw); err != nil {
		return nil, err
	}
	issues := make([]forge.Issue, 0, len(raw))
	for _, item := range raw {
		if item.PullRequest != nil || item.Number <= 0 || item.Title == "" {
			continue
		}
		issue := forge.Issue{Number: strconv.Itoa(item.Number), Title: item.Title, Body: truncate(item.Body, 4000), URL: item.HTMLURL, Labels: []string{}}
		for _, label := range item.Labels {
			issue.Labels = append(issue.Labels, label.Name)
		}
		issues = append(issues, issue)
	}
	return issues, nil
}

func (p *Provider) ListAdvisories(ctx context.Context, reference forge.RepoRef) ([]forge.Advisory, error) {
	segments, err := repositoryPath(reference)
	if err != nil {
		return nil, err
	}
	status, headers, body, err := p.request(ctx, http.MethodGet, append(segments, "dependabot", "alerts"), url.Values{"state": {"open"}, "per_page": {"100"}}, nil)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, responseError(status, headers)
	}
	var raw []struct {
		Number     int    `json:"number"`
		HTMLURL    string `json:"html_url"`
		Dependency struct {
			Package struct {
				Ecosystem string `json:"ecosystem"`
				Name      string `json:"name"`
			} `json:"package"`
			ManifestPath string `json:"manifest_path"`
		} `json:"dependency"`
		SecurityAdvisory struct {
			GHSAID   string `json:"ghsa_id"`
			Summary  string `json:"summary"`
			Severity string `json:"severity"`
		} `json:"security_advisory"`
		SecurityVulnerability struct {
			VulnerableVersionRange string `json:"vulnerable_version_range"`
			FirstPatchedVersion    *struct {
				Identifier string `json:"identifier"`
			} `json:"first_patched_version"`
		} `json:"security_vulnerability"`
	}
	if err := decode(body, &raw); err != nil {
		return nil, err
	}
	advisories := make([]forge.Advisory, 0, len(raw))
	for _, item := range raw {
		if item.SecurityAdvisory.GHSAID == "" || item.Dependency.Package.Name == "" {
			continue
		}
		advisory := forge.Advisory{ID: item.SecurityAdvisory.GHSAID, Package: item.Dependency.Package.Name, Ecosystem: item.Dependency.Package.Ecosystem, Manifest: item.Dependency.ManifestPath, Severity: item.SecurityAdvisory.Severity, Summary: truncate(item.SecurityAdvisory.Summary, 400), Vulnerable: item.SecurityVulnerability.VulnerableVersionRange, URL: item.HTMLURL}
		if item.SecurityVulnerability.FirstPatchedVersion != nil {
			advisory.Patched = item.SecurityVulnerability.FirstPatchedVersion.Identifier
		}
		advisories = append(advisories, advisory)
	}
	return advisories, nil
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
