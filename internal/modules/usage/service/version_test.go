package service

import "testing"

func TestCostPerMillionSpreadsTheWeeksSpendOverItsTokens(t *testing.T) {
	got := costPerMillion(26, "default_claude_max_20x", 12_000_000)
	if got < 0.99 || got > 1.01 {
		t.Fatalf("cost per million = %v, want 1.00", got)
	}
	if costPerMillion(26, "unknown", 1) != 0 || costPerMillion(26, "pro", 0) != 0 {
		t.Fatal("unknown tier or no tokens must give no rate")
	}
}

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
