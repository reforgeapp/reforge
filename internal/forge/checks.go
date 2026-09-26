package forge

import "sort"

func MissingChecks(target, head []Check, ignore func(string) bool) ([]string, bool) {
	passing := func(checks []Check) (map[string]bool, bool) {
		ok, failed, pending := map[string]bool{}, map[string]bool{}, false
		for _, c := range checks {
			if ignore(c.Name) {
				continue
			}
			switch c.Conclusion {
			case "success", "neutral", "skipped":
				ok[c.Name] = true
			case "":
				pending = true
			default:
				failed[c.Name] = true
			}
		}
		for name := range failed {
			delete(ok, name)
		}
		return ok, pending
	}
	want, _ := passing(target)
	have, pending := passing(head)
	missing := []string{}
	for name := range want {
		if !have[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing, pending
}
