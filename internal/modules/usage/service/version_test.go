package service

import "testing"

func TestNewestVersionComparesNumbersNotText(t *testing.T) {
	cases := []struct {
		versions []string
		want     string
	}{
		{[]string{"0.2.0", "0.10.0", "0.9.1"}, "0.10.0"},
		{[]string{"1.0.0-dev", "0.9.9"}, "1.0.0-dev"},
		{nil, ""},
	}
	for _, c := range cases {
		if got := newestVersion(c.versions); got != c.want {
			t.Fatalf("newestVersion(%v) = %q, want %q", c.versions, got, c.want)
		}
	}
}
