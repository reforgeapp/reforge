package connections

import "testing"

func TestRepositoryEndpoint(t *testing.T) {
	for _, c := range []struct {
		provider, endpoint string
		repository         bool
	}{
		{"github", "https://api.github.com", false},
		{"github", "https://ghe.example.test/api/v3", false},
		{"github", "https://github.com/acme/repo", true},
		{"github", "https://github.com", true},
		{"github", "https://api.github.com/repos/acme/repo", true},
		{"github", "https://ghe.example.test/acme/repo", true},
		{"github", "https://ghe.example.test/prefix/sub/api/v3", false},
		{"gitlab", "https://gitlab.com", false},
		{"gitlab", "https://gitlab.example.test/gitlab/api/v4", false},
		{"gitlab", "https://gl.example.test/prefix/sub/api/v1", false},
		{"gitlab", "https://gitlab.com/group/project", true},
		{"gitlab", "https://gitlab.com/group/api/v1", true},
		{"gitlab", "https://gitlab.example.test/group/project.git", true},
		{"gitea", "https://gitea.example.test/api/v1", false},
		{"gitea", "https://gitea.example.test/prefix/sub/api/v4", false},
		{"gitea", "https://gitea.example.test/owner/repo", true},
	} {
		if got := RepositoryEndpoint(c.provider, c.endpoint); got != c.repository {
			t.Errorf("%s %s: %v", c.provider, c.endpoint, got)
		}
	}
}
