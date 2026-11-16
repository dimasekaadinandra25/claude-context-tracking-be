package service

import (
	"encoding/json"
	"testing"
	"time"

	"claude-usage-server/internal/modules/usage/domain"
)

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

func TestPlanLimitsFromAFastClockAreCappedAtNow(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	raw, err := json.Marshal(domain.PlanLimits{
		SevenDay:   &domain.PlanWindow{UsedPercentage: 10, ResetsAt: now.Add(48 * time.Hour).Unix()},
		CapturedAt: now.Add(23 * time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	limits, ok := decodePlanLimits(raw, now)
	if !ok || limits.CapturedAt != now.Unix() {
		t.Fatalf("captured_at = %d ok=%v, want it capped at %d", limits.CapturedAt, ok, now.Unix())
	}
}
