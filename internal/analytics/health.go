package analytics

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// health scores a project 0-100 from weighted factors. Factors that don't
// apply (profile repos) or can't be measured are left out of the total
// rather than counted as zero, and the note says so.
func health(r *Report) Health {
	var fs []Factor
	add := func(name string, score, max float64, note string) {
		score = math.Max(0, math.Min(score, max))
		fs = append(fs, Factor{Name: name, Score: math.Round(score*10) / 10, Max: max, Note: note})
	}
	days := 0
	if r.Status.DaysSinceCommit != nil {
		days = *r.Status.DaysSinceCommit
	}
	product := r.Kind != "profile"

	// Activity
	actMax := 20.0
	if !product {
		actMax = 50
	}
	act := 0.0
	switch {
	case days <= 3:
		act = 1
	case days <= 7:
		act = 0.8
	case days <= 14:
		act = 0.5
	case days <= 30:
		act = 0.25
	}
	add("Activity", act*actMax, actMax, fmt.Sprintf("last commit %d days ago, %d commits in 30 days", days, r.Activity.Commits30d))

	if product {
		// CI
		switch {
		case !r.CI.Known:
			// unknown: excluded
		case r.CI.Runs == 0:
			add("CI", 0, 20, "no GitHub Actions runs in the lookback window")
		default:
			rate := 0.0
			if r.CI.SuccessRate != nil {
				rate = *r.CI.SuccessRate
			}
			s := rate * 15
			note := fmt.Sprintf("%.0f%% of runs pass", rate*100)
			if r.CI.DefaultStatus == "passing" {
				s += 5
				note += ", default branch green"
			} else if r.CI.DefaultStatus == "failing" {
				note += ", default branch failing (" + strings.Join(r.CI.Failing, ", ") + ")"
			}
			add("CI", s, 20, note)
		}

		// Pull request flow
		pr := 15.0
		note := fmt.Sprintf("%d open, %d stale, %d merged in 30 days", r.Pulls.Open, len(r.Pulls.Stale), r.Pulls.Merged30d)
		pr -= 3 * float64(len(r.Pulls.Stale))
		if r.Pulls.MedianMergeHours != nil && *r.Pulls.MedianMergeHours > 72 {
			pr -= 3
			note += fmt.Sprintf(", median merge %.0fh", *r.Pulls.MedianMergeHours)
		}
		add("Pull requests", pr, 15, note)

		// Issues
		if r.Issues.Known {
			s := 15.0
			s -= math.Min(8, float64(r.Issues.BugsOpen))
			old := 0
			for _, b := range r.Issues.Aging {
				if b.Label == "over 90 days" {
					old = b.Count
				}
			}
			s -= math.Min(4, float64(old)*0.5)
			if r.Issues.Net30d > 5 {
				s -= 3
			}
			add("Backlog", s, 15, fmt.Sprintf("%d open (%d bugs), net %+d in 30 days", r.Issues.Open, r.Issues.BugsOpen, r.Issues.Net30d))
		}

		// Security
		if r.Security.DependabotKnown || r.Security.CodeScanKnown || r.Security.SecretsKnown {
			d, c := r.Security.Dependabot, r.Security.CodeScanning
			crit, high, med := d.Critical+c.Critical, d.High+c.High, d.Medium+c.Medium
			s := 15 - 8*float64(crit) - 4*float64(high) - float64(med)
			if r.Security.SecretsOpen > 0 {
				s = 0
			}
			add("Security", s, 15, fmt.Sprintf("%d critical, %d high, %d medium alerts; %d exposed secrets", crit, high, med, r.Security.SecretsOpen))
		}

		// Delivery cadence
		switch {
		case r.Releases.DaysSince != nil && *r.Releases.DaysSince <= 30:
			add("Delivery", 5, 5, "released "+r.Releases.LatestTag+fmt.Sprintf(" %d days ago", *r.Releases.DaysSince))
		case r.Releases.DaysSince != nil && *r.Releases.DaysSince <= 90:
			add("Delivery", 3, 5, fmt.Sprintf("last release %d days ago", *r.Releases.DaysSince))
		case r.CI.DefaultStatus == "passing" && days <= 7:
			add("Delivery", 3, 5, "no tagged releases, but the default branch builds green")
		default:
			add("Delivery", 0, 5, "no recent releases")
		}
	}

	// Docs & repo setup
	docMax := 10.0
	if !product {
		docMax = 50
	}
	switch {
	case r.Hygiene.Known && product:
		add("Docs & setup", float64(r.Hygiene.CommunityHealth)/100*docMax, docMax, fmt.Sprintf("GitHub community health %d%%", r.Hygiene.CommunityHealth))
	case r.Hygiene.Known:
		s := 0.0
		if r.Hygiene.Readme {
			s = docMax
		}
		add("Docs & setup", s, docMax, map[bool]string{true: "README present", false: "no README"}[r.Hygiene.Readme])
	}

	var got, max float64
	for _, f := range fs {
		got += f.Score
		max += f.Max
	}
	score := 0
	if max > 0 {
		score = int(math.Round(got / max * 100))
	}
	h := Health{Score: score, Factors: fs}
	switch {
	case score >= 85:
		h.Grade = "A"
	case score >= 70:
		h.Grade = "B"
	case score >= 55:
		h.Grade = "C"
	case score >= 40:
		h.Grade = "D"
	default:
		h.Grade = "F"
	}
	switch {
	case score >= 70:
		h.RAG = "green"
	case score >= 50:
		h.RAG = "amber"
	default:
		h.RAG = "red"
	}
	// A failing default branch or an exposed secret is never green.
	if h.RAG == "green" && (r.CI.DefaultStatus == "failing" || r.Security.SecretsOpen > 0) {
		h.RAG = "amber"
	}
	return h
}

func severityRank(s string) int {
	switch s {
	case "high":
		return 0
	case "medium":
		return 1
	}
	return 2
}

// risks lists what a tech lead or PM should act on, most severe first.
func risks(r *Report) []Risk {
	out := []Risk{}
	add := func(sev, area, title, detail, url string) {
		out = append(out, Risk{Project: r.Name, Severity: sev, Area: area, Title: title, Detail: detail, URL: url})
	}
	product := r.Kind != "profile"
	days := 0
	if r.Status.DaysSinceCommit != nil {
		days = *r.Status.DaysSinceCommit
	}

	if r.Security.SecretsOpen > 0 {
		add("high", "security", fmt.Sprintf("%d exposed secret(s) detected", r.Security.SecretsOpen),
			"GitHub secret scanning found credentials in the repo. Rotate them now; deleting the commit does not revoke them.", r.URL+"/security/secret-scanning")
	}
	if crit := r.Security.Dependabot.Critical + r.Security.CodeScanning.Critical; crit > 0 {
		add("high", "security", fmt.Sprintf("%d critical security alert(s)", crit), "Open critical Dependabot or code-scanning alerts.", r.URL+"/security")
	} else if high := r.Security.Dependabot.High + r.Security.CodeScanning.High; high > 0 {
		add("medium", "security", fmt.Sprintf("%d high-severity security alert(s)", high), "Open high-severity Dependabot or code-scanning alerts.", r.URL+"/security")
	}
	if r.CI.DefaultStatus == "failing" {
		u := ""
		if r.CI.LastDefaultRun != nil {
			u = r.CI.LastDefaultRun.URL
		}
		add("high", "quality", "Default branch build is failing",
			"Latest run failed on "+r.Branch+" for: "+strings.Join(r.CI.Failing, ", ")+". Anything merged now ships on a red build.", u)
	}
	if product && r.CI.Known && r.CI.Runs == 0 && r.Status.State != "empty" {
		add("medium", "quality", "No CI runs", fmt.Sprintf("No GitHub Actions runs in the last %d days. Changes merge without automated tests or builds.", r.Lookback), r.URL+"/actions")
	} else if r.CI.SuccessRate != nil && *r.CI.SuccessRate < 0.7 && r.CI.Runs >= 5 {
		add("medium", "quality", fmt.Sprintf("Flaky or failing CI (%.0f%% pass rate)", *r.CI.SuccessRate*100), "Fewer than 7 in 10 CI runs pass. Check which workflows fail most.", r.URL+"/actions")
	}
	if product {
		switch r.Status.State {
		case "stalled":
			add("medium", "delivery", r.Status.Label, "No commits on the default branch for over a month. Confirm whether the project is paused or blocked.", r.URL+"/commits")
		case "slowing":
			add("low", "delivery", r.Status.Label, "Commit activity has dropped off.", r.URL+"/commits")
		}
		if r.Activity.Trend == "down" && r.Activity.TrendPct != nil && r.Status.State == "active" {
			add("low", "delivery", fmt.Sprintf("Velocity down %d%% vs previous 4 weeks", -*r.Activity.TrendPct), "Fewer commits in the last 4 weeks than the 4 before.", "")
		}
	} else if days > 90 {
		add("low", "delivery", "Profile not updated in 3 months", "Recruiters and clients read this first; keep current projects listed.", r.URL)
	}
	if n := len(r.Pulls.Stale); n > 0 {
		sev := "low"
		if n >= 3 {
			sev = "medium"
		}
		p := r.Pulls.Stale[0]
		add(sev, "process", fmt.Sprintf("%d pull request(s) idle 7+ days", n),
			fmt.Sprintf("Oldest: #%d %q, idle %d days. Review, merge or close.", p.Number, p.Title, p.IdleDays), p.URL)
	}
	if r.Pulls.MedianMergeHours != nil && *r.Pulls.MedianMergeHours > 72 {
		add("low", "process", fmt.Sprintf("Slow reviews: median %.0fh to merge", *r.Pulls.MedianMergeHours), "Pull requests take over 3 days to merge.", r.URL+"/pulls")
	}
	for _, m := range r.Progress.Milestones {
		if m.Overdue {
			add("medium", "delivery", "Milestone overdue: "+m.Title, fmt.Sprintf("%d issues still open, due %d days ago.", m.Open, -*m.DaysToDue), m.URL)
		} else if m.State == "open" && m.OnTrack != nil && !*m.OnTrack && m.DaysToDue != nil {
			detail := fmt.Sprintf("%d open issues, due in %d days", m.Open, *m.DaysToDue)
			if m.ForecastDays != nil {
				detail += fmt.Sprintf("; at the current closing rate it needs about %d days", *m.ForecastDays)
			} else {
				detail += "; nothing in it was closed in the last 30 days"
			}
			add("medium", "delivery", "Milestone at risk: "+m.Title, detail+".", m.URL)
		}
	}
	if r.Issues.Known {
		if r.Issues.Net30d > 5 {
			add("medium", "delivery", fmt.Sprintf("Backlog growing (+%d in 30 days)", r.Issues.Net30d), fmt.Sprintf("%d issues opened vs %d closed.", r.Issues.Opened30d, r.Issues.Closed30d), r.URL+"/issues")
		}
		if r.Issues.BugsOpen >= 5 {
			add("medium", "quality", fmt.Sprintf("%d open bugs", r.Issues.BugsOpen), "Issues labelled bug are piling up.", r.URL+"/issues?q=is%3Aopen+label%3Abug")
		}
		if r.Issues.Open >= 5 && r.Issues.Unlabeled*2 > r.Issues.Open {
			add("low", "process", "Most open issues are unlabelled", fmt.Sprintf("%d of %d open issues have no label, so bugs and features can't be told apart.", r.Issues.Unlabeled, r.Issues.Open), r.URL+"/issues")
		}
	}
	if product && r.Activity.CommitsTotal >= 10 {
		if r.Activity.HumanAuthors == 1 {
			add("low", "people", "Bus factor of 1", "Only one person has committed recently. Document setup and decisions so someone else could pick it up.", "")
		}
		if r.Activity.Authorship.AIShare >= 0.6 {
			add("low", "process", fmt.Sprintf("%.0f%% of commits are AI-assisted", r.Activity.Authorship.AIShare*100), "Make sure AI-written changes go through pull-request review and CI before merging.", "")
		}
	}
	if product && r.Hygiene.DefaultProtected != nil && !*r.Hygiene.DefaultProtected && r.Activity.CommitsTotal > 0 {
		add("low", "process", "Default branch isn't protected", "Anyone with write access can push straight to "+r.Branch+". Turn on branch protection to require PRs and passing CI.", r.URL+"/settings/branches")
	}
	if r.Hygiene.Known && !r.Hygiene.Readme {
		add("low", "process", "No README", "Add a README with what the project is and how to run it.", r.URL)
	}
	if len(r.DataGaps) > 0 {
		add("low", "data", fmt.Sprintf("%d data source(s) not visible", len(r.DataGaps)), "Some metrics are unknown because the token can't read: "+strings.Join(shortGaps(r.DataGaps), ", ")+".", "")
	}
	sort.SliceStable(out, func(i, j int) bool { return severityRank(out[i].Severity) < severityRank(out[j].Severity) })
	return out
}

func shortGaps(g []string) []string {
	out := make([]string, 0, len(g))
	for _, s := range g {
		if i := strings.Index(s, ":"); i > 0 {
			s = s[:i]
		}
		out = append(out, s)
	}
	return out
}
