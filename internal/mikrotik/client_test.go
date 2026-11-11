package mikrotik

import "testing"

func TestParseRouterOSUptime(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"33s", 33},
		{"5m33s", 333},
		{"12h5m33s", 43533},
		{"3d12h5m33s", 302733},
		{"4w3d12h5m33s", 2721933},
		{"", 0},
	}
	for _, c := range cases {
		if got := parseRouterOSUptime(c.in); got != c.want {
			t.Errorf("parseRouterOSUptime(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
