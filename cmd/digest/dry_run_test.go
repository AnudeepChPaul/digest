package main

import "testing"

func TestIsDryRunSpotsTheFlagAnywhereInTheCommand(t *testing.T) {
	cases := map[string]struct {
		args []string
		want bool
	}{
		"double dash": {[]string{"janitor", "--dry-run", "--root", "~"}, true},
		"single dash": {[]string{"repo-sync", "-dry-run"}, true},
		"with value":  {[]string{"branch-reaper", "--dry-run=true"}, true},
		"real run":    {[]string{"janitor", "--root", "~"}, false},
		"tui":         {nil, false},
		"false value": {[]string{"janitor", "--dry-run=false"}, false},
	}
	for name, c := range cases {
		if got := isDryRun(c.args); got != c.want {
			t.Errorf("%s: isDryRun(%v) = %v, want %v", name, c.args, got, c.want)
		}
	}
}
