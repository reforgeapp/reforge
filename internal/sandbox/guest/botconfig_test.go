package guest

import (
	"testing"
	"testing/fstest"
)

func TestValidateBotConfig(t *testing.T) {
	grouped := "version: 2\nupdates:\n  - package-ecosystem: gomod\n    directory: /\n    schedule: {interval: weekly}\n    groups: {all: {patterns: ['*']}}\n"
	for name, tc := range map[string]struct {
		files fstest.MapFS
		valid bool
	}{
		"missing":   {fstest.MapFS{}, false},
		"ungrouped": {fstest.MapFS{".github/dependabot.yml": {Data: []byte("version: 2\nupdates:\n  - package-ecosystem: gomod\n    directory: /\n    schedule: {interval: weekly}\n")}}, false},
		"grouped":   {fstest.MapFS{".github/dependabot.yml": {Data: []byte(grouped)}}, true},
		"renovate":  {fstest.MapFS{"renovate.json": {Data: []byte(`{"extends":["config:recommended"]}`)}}, true},
		"broken":    {fstest.MapFS{"renovate.json": {Data: []byte(`{"extends":`)}}, false},
	} {
		if problems := ValidateBotConfig(tc.files); (len(problems) == 0) != tc.valid {
			t.Errorf("%s: %v", name, problems)
		}
	}
}
