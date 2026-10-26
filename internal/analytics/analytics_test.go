package analytics

import (
	"testing"
	"time"

	gh "github.com/KING-CYBERTON/project-pulse/internal/github"
)

func commit(name, login, typ, msg string) gh.Commit {
	var c gh.Commit
	c.Commit.Author.Name = name
	c.Commit.Message = msg
	if login != "" {
		c.Author = &gh.User{Login: login, Type: typ}
	}
	return c
}

func TestClassify(t *testing.T) {
	cases := []struct {
		c    gh.Commit
		want string
	}{
		{commit("Kimani", "KING-CYBERTON", "User", "Fix bug"), "human"},
		{commit("Claude", "", "", "Add tests"), "ai"},
		{commit("Kimani", "KING-CYBERTON", "User", "Feature\n\nCo-Authored-By: Claude <noreply@anthropic.com>"), "ai"},
		{commit("dependabot[bot]", "dependabot[bot]", "Bot", "Bump"), "bot"},
		{commit("github-actions", "github-actions[bot]", "Bot", "chore"), "bot"},
	}
	for _, c := range cases {
		if got := Classify(c.c); got != c.want {
			t.Errorf("Classify(%q) = %s, want %s", c.c.Commit.Author.Name, got, c.want)
		}
	}
}

func TestWeekStartIsMonday(t *testing.T) {
	sun := time.Date(2026, 10, 25, 23, 0, 0, 0, time.UTC)
	if got := weekStart(sun).Format("2006-01-02"); got != "2026-10-19" {
		t.Errorf("weekStart(Sun 25 Oct) = %s, want 2026-10-19", got)
	}
	mon := time.Date(2026, 10, 26, 1, 0, 0, 0, time.UTC)
	if got := weekStart(mon).Format("2006-01-02"); got != "2026-10-26" {
		t.Errorf("weekStart(Mon 26 Oct) = %s", got)
	}
}

func TestPercentile(t *testing.T) {
	if p := percentile(nil, .5); p != nil {
		t.Error("empty should be nil")
	}
	if p := percentile([]float64{4, 1, 3, 2}, .5); *p != 2.5 {
		t.Errorf("median = %v", *p)
	}
}

func TestProfileHealthIgnoresCIAndPRs(t *testing.T) {
	days := 2
	r := &Report{Kind: "profile", Status: Status{DaysSinceCommit: &days}, Hygiene: Hygiene{Known: true, Readme: true}}
	h := health(r)
	if h.Score != 100 || h.RAG != "green" {
		t.Errorf("fresh profile with README should score 100 green, got %d %s (%+v)", h.Score, h.RAG, h.Factors)
	}
	for _, f := range h.Factors {
		if f.Name == "CI" || f.Name == "Pull requests" {
			t.Errorf("profile scored on %s", f.Name)
		}
	}
}

func TestUnknownCIIsExcludedNotZero(t *testing.T) {
	days := 1
	r := &Report{Kind: "product", Status: Status{DaysSinceCommit: &days}, CI: CI{Known: false}}
	for _, f := range health(r).Factors {
		if f.Name == "CI" {
			t.Error("CI factor should be excluded when CI data is unavailable")
		}
	}
}

func TestStalledProductRaisesRisk(t *testing.T) {
	days := 45
	r := &Report{Name: "x", Kind: "product", Status: Status{State: "stalled", Label: "Stalled: no commits for 45 days", DaysSinceCommit: &days}, CI: CI{Known: true}}
	found := false
	for _, k := range risks(r) {
		if k.Area == "delivery" && k.Severity == "medium" {
			found = true
		}
	}
	if !found {
		t.Error("expected a medium delivery risk for a stalled product")
	}
}
