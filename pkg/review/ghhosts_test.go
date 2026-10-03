package review

import (
	"reflect"
	"testing"
)

func TestParseAuthHostsKeepsHostsThatFailedTheCheck(t *testing.T) {
	output := `git.example.com
  ✓ Logged in to git.example.com account someone (keyring)
  - Active account: true

other.example.com
  X Failed to log in to other.example.com account someone (keyring)
  - The token in keyring is invalid.

slow.example.com
  X Timeout trying to log in to slow.example.com account someone (keyring)
`
	hosts := parseAuthHosts(output)
	want := []string{"git.example.com", "other.example.com", "slow.example.com"}
	if !reflect.DeepEqual(hosts, want) {
		t.Errorf("hosts = %v, want %v", hosts, want)
	}
}
