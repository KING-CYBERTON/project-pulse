package analytics

import (
	"math"
	"sort"
	"time"
)

// Summarize gives the one-row view of a project.
func Summarize(r Report) Summary {
	s := Summary{
		Name: r.Name, Repo: r.Repo, Kind: r.Kind, Description: r.Description, URL: r.URL, Private: r.Private,
		State: r.Status.State, StateLabel: r.Status.Label, Score: r.Health.Score, Grade: r.Health.Grade, RAG: r.Health.RAG,
		LastCommitAt: r.Status.LastCommitAt, Commits7d: r.Activity.Commits7d, Commits30d: r.Activity.Commits30d,
		Trend: r.Activity.Trend, Spark: []int{}, OpenPRs: r.Pulls.Open, StalePRs: len(r.Pulls.Stale),
		Merged30d: r.Pulls.Merged30d, OpenIssues: r.Issues.Open, CIStatus: r.CI.DefaultStatus,
		CISuccess: r.CI.SuccessRate, Milestone: r.Progress.Current, RiskCount: len(r.Risks), Error: r.Error,
	}
	for _, w := range r.Activity.Weekly {
		s.Spark = append(s.Spark, w.Total)
	}
	if len(r.Risks) > 0 {
		top := r.Risks[0]
		s.TopRisk = &top
	}
	return s
}

// BuildPortfolio rolls reports up into one view.
func BuildPortfolio(reports []Report, lookback int, now time.Time) Portfolio {
	p := Portfolio{GeneratedAt: now.UTC(), Lookback: lookback, Projects: []Summary{}, Attention: []Risk{}, Weekly: []PortWeek{}, Focus: []FocusShare{}}
	weeks := map[string]*PortWeek{}
	var order []string
	total30 := 0
	for _, r := range reports {
		s := Summarize(r)
		p.Projects = append(p.Projects, s)
		t := &p.Totals
		t.Projects++
		switch r.Health.RAG {
		case "green":
			t.Green++
		case "amber":
			t.Amber++
		default:
			t.Red++
		}
		t.Commits7d += r.Activity.Commits7d
		t.Commits30d += r.Activity.Commits30d
		t.OpenPRs += r.Pulls.Open
		t.StalePRs += len(r.Pulls.Stale)
		t.Merged30d += r.Pulls.Merged30d
		t.OpenIssues += r.Issues.Open
		t.OpenBugs += r.Issues.BugsOpen
		if r.CI.DefaultStatus == "failing" {
			t.FailingCI++
		}
		t.SecurityAlerts += r.Security.Dependabot.Total + r.Security.CodeScanning.Total + r.Security.SecretsOpen
		for _, k := range r.Risks {
			if k.Severity == "high" {
				t.HighRisks++
			}
			p.Attention = append(p.Attention, k)
		}
		for _, w := range r.Activity.Weekly {
			pw := weeks[w.Start]
			if pw == nil {
				pw = &PortWeek{Start: w.Start, ByProject: map[string]int{}}
				weeks[w.Start] = pw
				order = append(order, w.Start)
			}
			pw.Total += w.Total
			pw.ByProject[r.Name] = w.Total
		}
		total30 += r.Activity.Commits30d
	}
	sort.Strings(order)
	for _, k := range order {
		p.Weekly = append(p.Weekly, *weeks[k])
	}
	for _, r := range reports {
		share := 0.0
		if total30 > 0 {
			share = math.Round(float64(r.Activity.Commits30d)/float64(total30)*1000) / 1000
		}
		p.Focus = append(p.Focus, FocusShare{Name: r.Name, Commits: r.Activity.Commits30d, Share: share})
	}
	sort.SliceStable(p.Focus, func(i, j int) bool { return p.Focus[i].Commits > p.Focus[j].Commits })
	sort.SliceStable(p.Attention, func(i, j int) bool {
		return severityRank(p.Attention[i].Severity) < severityRank(p.Attention[j].Severity)
	})
	if len(p.Attention) > 30 {
		p.Attention = p.Attention[:30]
	}
	// Worst health first so problems surface at the top.
	sort.SliceStable(p.Projects, func(i, j int) bool {
		ri, rj := ragRank(p.Projects[i].RAG), ragRank(p.Projects[j].RAG)
		if ri != rj {
			return ri < rj
		}
		return p.Projects[i].Score < p.Projects[j].Score
	})
	return p
}

func ragRank(r string) int {
	switch r {
	case "red":
		return 0
	case "amber":
		return 1
	}
	return 2
}
