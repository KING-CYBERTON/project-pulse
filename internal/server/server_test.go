package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/KING-CYBERTON/project-pulse/internal/analytics"
	"github.com/KING-CYBERTON/project-pulse/internal/config"
	gh "github.com/KING-CYBERTON/project-pulse/internal/github"
)

var now = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

func ts(daysAgo float64) string {
	return now.Add(-time.Duration(daysAgo * 24 * float64(time.Hour))).Format(time.RFC3339)
}

// fakeGitHub serves a realistic slice of the GitHub API for acme/app and
// answers 404 for anything else. Security endpoints return 403 to exercise
// the "not visible to this token" path.
func fakeGitHub(t *testing.T) *httptest.Server {
	t.Helper()
	routes := map[string]string{
		"/repos/acme/app":           `{"name":"app","full_name":"acme/app","description":"Demo app","private":true,"default_branch":"main","language":"Go","size":1200,"html_url":"https://github.com/acme/app","created_at":"` + ts(200) + `","pushed_at":"` + ts(0.5) + `"}`,
		"/repos/acme/app/languages": `{"Go":7000,"TypeScript":3000}`,
		"/repos/acme/app/commits": `[
			{"sha":"a1","html_url":"https://github.com/acme/app/commit/a1","commit":{"message":"Add billing\n\nCo-Authored-By: Claude <noreply@anthropic.com>","author":{"name":"Kimani","email":"k@x.dev","date":"` + ts(0.5) + `"}},"author":{"login":"KING-CYBERTON","type":"User"}},
			{"sha":"a2","commit":{"message":"Fix login","author":{"name":"Claude","email":"noreply@anthropic.com","date":"` + ts(1) + `"}},"author":null},
			{"sha":"a3","commit":{"message":"Bump x","author":{"name":"dependabot[bot]","email":"b@x","date":"` + ts(3) + `"}},"author":{"login":"dependabot[bot]","type":"Bot"}},
			{"sha":"a4","commit":{"message":"Init","author":{"name":"Kimani","email":"k@x.dev","date":"` + ts(40) + `"}},"author":{"login":"KING-CYBERTON","type":"User"}}
		]`,
		"/repos/acme/app/pulls?state=open": `[
			{"number":7,"title":"Old feature","state":"open","html_url":"https://github.com/acme/app/pull/7","user":{"login":"kim"},"created_at":"` + ts(20) + `","updated_at":"` + ts(12) + `"},
			{"number":9,"title":"Fresh fix","state":"open","html_url":"https://github.com/acme/app/pull/9","user":{"login":"kim"},"created_at":"` + ts(1) + `","updated_at":"` + ts(1) + `"}
		]`,
		"/repos/acme/app/pulls?state=closed": `[
			{"number":5,"title":"Ship billing","state":"closed","html_url":"https://github.com/acme/app/pull/5","user":{"login":"kim"},"created_at":"` + ts(4) + `","updated_at":"` + ts(3) + `","closed_at":"` + ts(3) + `","merged_at":"` + ts(3) + `"},
			{"number":4,"title":"Abandoned","state":"closed","user":{"login":"kim"},"created_at":"` + ts(10) + `","updated_at":"` + ts(9) + `","closed_at":"` + ts(9) + `","merged_at":null}
		]`,
		"/repos/acme/app/issues?state=open": `[
			{"number":11,"title":"Crash on save","state":"open","labels":[{"name":"bug"}],"assignees":[],"created_at":"` + ts(100) + `","updated_at":"` + ts(5) + `"},
			{"number":12,"title":"Add export","state":"open","labels":[],"assignees":[{"login":"kim"}],"created_at":"` + ts(2) + `","updated_at":"` + ts(2) + `","milestone":{"title":"MVP"}},
			{"number":9,"title":"a PR","state":"open","pull_request":{},"created_at":"` + ts(1) + `","updated_at":"` + ts(1) + `"}
		]`,
		"/repos/acme/app/issues?state=closed": `[
			{"number":10,"title":"Signup","state":"closed","labels":[],"created_at":"` + ts(15) + `","updated_at":"` + ts(5) + `","closed_at":"` + ts(5) + `","milestone":{"title":"MVP"}}
		]`,
		"/repos/acme/app/milestones": `[{"number":1,"title":"MVP","state":"open","open_issues":1,"closed_issues":3,"due_on":"` + now.AddDate(0, 0, 20).Format(time.RFC3339) + `","html_url":"https://github.com/acme/app/milestone/1"}]`,
		"/repos/acme/app/actions/runs": `{"total_count":3,"workflow_runs":[
			{"id":3,"name":"ci","head_branch":"main","event":"push","status":"completed","conclusion":"failure","html_url":"https://github.com/acme/app/actions/runs/3","created_at":"` + ts(0.4) + `","updated_at":"` + ts(0.39) + `","run_started_at":"` + ts(0.4) + `"},
			{"id":2,"name":"ci","head_branch":"main","event":"push","status":"completed","conclusion":"success","created_at":"` + ts(1) + `","updated_at":"` + ts(0.99) + `","run_started_at":"` + ts(1) + `"},
			{"id":1,"name":"deploy","head_branch":"main","event":"push","status":"completed","conclusion":"success","created_at":"` + ts(1) + `","updated_at":"` + ts(0.98) + `","run_started_at":"` + ts(1) + `"}
		]}`,
		"/repos/acme/app/releases":          `[{"tag_name":"v0.3.0","html_url":"https://github.com/acme/app/releases/v0.3.0","published_at":"` + ts(10) + `"}]`,
		"/repos/acme/app/branches":          `[{"name":"main","protected":false},{"name":"feat/x","protected":false}]`,
		"/repos/acme/app/community/profile": `{"health_percentage":57,"files":{"readme":{"html_url":"x"},"license":null}}`,
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing auth header on %s", r.URL.Path)
		}
		w.Header().Set("X-RateLimit-Remaining", "4990")
		w.Header().Set("X-RateLimit-Limit", "5000")
		key := r.URL.Path
		if st := r.URL.Query().Get("state"); st != "" && (strings.HasSuffix(key, "/pulls") || strings.HasSuffix(key, "/issues")) {
			key += "?state=" + st
		}
		if strings.Contains(key, "alerts") {
			w.WriteHeader(http.StatusForbidden)
			io.WriteString(w, `{"message":"Resource not accessible by personal access token"}`)
			return
		}
		body, ok := routes[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"message":"Not Found"}`)
			return
		}
		io.WriteString(w, body)
	}))
}

func newTestServer(t *testing.T) *httptest.Server {
	gh := fakeGitHub(t)
	t.Cleanup(gh.Close)
	cfg := &config.Config{
		Token: "test-token", APIBase: gh.URL, CacheTTL: time.Minute, LookbackDays: 90,
		Projects: []config.Project{
			{Name: "app", Repo: "acme/app", Kind: "product"},
			{Name: "ghost", Repo: "acme/ghost", Kind: "product"},
		},
	}
	client := newClient(cfg)
	svc := NewService(cfg, client, func() time.Time { return now })
	srv := httptest.NewServer(New(svc, client))
	t.Cleanup(srv.Close)
	return srv
}

func newClient(cfg *config.Config) *gh.Client { return gh.New(cfg.APIBase, cfg.Token) }

func getJSON(t *testing.T, url string, out any) int {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			t.Fatalf("decode %s: %v", url, err)
		}
	}
	return res.StatusCode
}

func TestProjectReport(t *testing.T) {
	srv := newTestServer(t)
	var r analytics.Report
	if code := getJSON(t, srv.URL+"/api/projects/app", &r); code != 200 {
		t.Fatalf("status %d", code)
	}
	checks := []struct {
		name      string
		got, want any
	}{
		{"state", r.Status.State, "active"},
		{"commits total", r.Activity.CommitsTotal, 4},
		{"commits 7d", r.Activity.Commits7d, 3},
		{"commits 30d", r.Activity.Commits30d, 3},
		{"human", r.Activity.Authorship.Human, 1},
		{"ai", r.Activity.Authorship.AI, 2},
		{"bot", r.Activity.Authorship.Bot, 1},
		{"open PRs", r.Pulls.Open, 2},
		{"stale PRs", len(r.Pulls.Stale), 1},
		{"merged 30d", r.Pulls.Merged30d, 1},
		{"closed unmerged", r.Pulls.ClosedUnmerged30, 1},
		{"open issues (PRs excluded)", r.Issues.Open, 2},
		{"bugs", r.Issues.BugsOpen, 1},
		{"closed 30d", r.Issues.Closed30d, 1},
		{"ci default", r.CI.DefaultStatus, "failing"},
		{"ci failing", strings.Join(r.CI.Failing, ","), "ci"},
		{"latest release", r.Releases.LatestTag, "v0.3.0"},
		{"milestone pct", r.Progress.Current.Percent, 75.0},
		{"community", r.Hygiene.CommunityHealth, 57},
		{"security unknown", r.Security.DependabotKnown, false},
		{"languages", r.Languages[0].Name, "Go"},
		{"rag never green when CI failing", r.Health.RAG != "green", true},
	}
	for _, c := range checks {
		if fmt.Sprint(c.got) != fmt.Sprint(c.want) {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if r.CI.SuccessRate == nil || *r.CI.SuccessRate < 0.66 || *r.CI.SuccessRate > 0.67 {
		t.Errorf("ci success rate = %v, want 0.667", r.CI.SuccessRate)
	}
	titles := []string{}
	for _, k := range r.Risks {
		titles = append(titles, k.Title)
	}
	joined := strings.Join(titles, " | ")
	for _, want := range []string{"Default branch build is failing", "idle 7+ days", "data source(s) not visible", "Default branch isn't protected"} {
		if !strings.Contains(joined, want) {
			t.Errorf("risks missing %q; got %s", want, joined)
		}
	}
	if r.Risks[0].Severity != "high" {
		t.Errorf("risks not sorted by severity: first is %q", r.Risks[0].Severity)
	}
}

func TestPortfolioAndFailedProject(t *testing.T) {
	srv := newTestServer(t)
	var p analytics.Portfolio
	if code := getJSON(t, srv.URL+"/api/portfolio", &p); code != 200 {
		t.Fatalf("status %d", code)
	}
	if p.Totals.Projects != 2 || p.Totals.FailingCI != 1 {
		t.Errorf("totals = %+v", p.Totals)
	}
	if p.Projects[0].Name != "ghost" || p.Projects[0].Error == "" {
		t.Errorf("unreachable project should sort first with an error, got %+v", p.Projects[0])
	}
	if len(p.Weekly) != 12 {
		t.Errorf("weekly buckets = %d, want 12", len(p.Weekly))
	}
}

func TestEndpoints(t *testing.T) {
	srv := newTestServer(t)
	cases := []struct {
		path string
		code int
	}{
		{"/api/health", 200},
		{"/api/projects", 200},
		{"/api/projects/app/ci", 200},
		{"/api/projects/app/nope", 404},
		{"/api/projects/missing", 404},
		{"/api/risks?severity=high", 200},
		{"/api/report.md?days=99", 400},
	}
	for _, c := range cases {
		if code := getJSON(t, srv.URL+c.path, nil); code != c.code {
			t.Errorf("%s = %d, want %d", c.path, code, c.code)
		}
	}
	res, err := http.Get(srv.URL + "/api/report.md")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	md := string(b)
	for _, want := range []string{"# Project status", "| [app]", "Default branch build is failing", "#5 Ship billing"} {
		if !strings.Contains(md, want) {
			t.Errorf("report missing %q", want)
		}
	}
	res, err = http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	b, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(b), "<title>Project Pulse</title>") {
		t.Error("dashboard page not served at /")
	}
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/refresh", nil)
	res, err = http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != 200 {
		t.Errorf("refresh failed: %v %v", err, res.StatusCode)
	}
}
