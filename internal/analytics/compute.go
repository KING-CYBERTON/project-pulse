package analytics

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/KING-CYBERTON/project-pulse/internal/config"
	gh "github.com/KING-CYBERTON/project-pulse/internal/github"
)

const (
	staleAfterDays = 7
	weeksShown     = 12
	commitCap      = 1000 // FetchRepo reads at most 10 pages of 100
)

// Analyze builds the full report for one project.
func Analyze(p config.Project, d *gh.RepoData, lookback int, now time.Time) Report {
	now = now.UTC()
	r := Report{
		Name:        p.Name,
		Repo:        p.Repo,
		Kind:        p.Kind,
		Description: p.Description,
		Owner:       p.Owner,
		URL:         d.Repo.HTMLURL,
		Private:     d.Repo.Private,
		Branch:      d.Repo.DefaultBranch,
		GeneratedAt: now,
		Lookback:    lookback,
	}
	if r.Description == "" {
		r.Description = d.Repo.Description
	}
	r.Languages = languages(d.Languages)
	r.DataGaps = gaps(d.Unavailable)

	r.Activity = activity(d, now)
	r.Status = status(d, now)
	r.Pulls = pulls(d, now)
	r.CI = ci(d, lookback, now)
	r.Releases = releases(d, lookback, now)
	r.Issues = issues(d, now)
	r.Progress = progress(d, now)
	r.Hygiene = hygiene(d)
	r.Security = security(d)
	fillWeeklyFlow(&r.Activity, d)

	r.Health = health(&r)
	r.Risks = risks(&r)
	return r
}

// Failed builds a placeholder report for a project that couldn't be fetched.
func Failed(p config.Project, err error, lookback int, now time.Time) Report {
	msg := err.Error()
	if gh.IsPermission(err) {
		msg = "GitHub returned 'not found' or 'forbidden' for " + p.Repo + ". Check the repo name and that your token has access to it."
	}
	return Report{
		Name: p.Name, Repo: p.Repo, Kind: p.Kind, Description: p.Description, Owner: p.Owner,
		URL: "https://github.com/" + p.Repo, GeneratedAt: now.UTC(), Lookback: lookback,
		Status: Status{State: "unknown", Label: "Couldn't load"},
		Health: Health{Grade: "?", RAG: "red"},
		Risks:  []Risk{{Project: p.Name, Severity: "high", Area: "data", Title: "Project data couldn't be loaded", Detail: msg}},
		Error:  msg,
	}
}

// ---- helpers ---------------------------------------------------------------

func daysBetween(now, t time.Time) int {
	if t.IsZero() {
		return 0
	}
	d := int(now.Sub(t).Hours() / 24)
	if d < 0 {
		return 0
	}
	return d
}

func weekStart(t time.Time) time.Time {
	t = t.UTC()
	wd := (int(t.Weekday()) + 6) % 7 // Monday = 0
	y, m, dd := t.Date()
	return time.Date(y, m, dd-wd, 0, 0, 0, 0, time.UTC)
}

func percentile(xs []float64, p float64) *float64 {
	if len(xs) == 0 {
		return nil
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	if len(s) == 1 {
		v := s[0]
		return &v
	}
	pos := p * float64(len(s)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	v := s[lo] + (s[hi]-s[lo])*(pos-float64(lo))
	v = math.Round(v*10) / 10
	return &v
}

func ratio(num, den int) *float64 {
	if den == 0 {
		return nil
	}
	v := math.Round(float64(num)/float64(den)*1000) / 1000
	return &v
}

func intp(v int) *int { return &v }

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if len(s) > 120 {
		s = s[:117] + "..."
	}
	return s
}

func hasLabel(labels []gh.Label, sub string) bool {
	for _, l := range labels {
		if strings.Contains(strings.ToLower(l.Name), sub) {
			return true
		}
	}
	return false
}

var sourceNames = map[string]string{
	"languages":       "Languages",
	"commits":         "Commits",
	"pulls_open":      "Open pull requests",
	"pulls_closed":    "Closed pull requests",
	"issues_open":     "Open issues",
	"issues_closed":   "Closed issues",
	"milestones":      "Milestones",
	"actions":         "CI runs (GitHub Actions)",
	"releases":        "Releases",
	"branches":        "Branches",
	"community":       "Community profile (README, license, templates)",
	"dependabot":      "Dependabot alerts",
	"code_scanning":   "Code scanning alerts",
	"secret_scanning": "Secret scanning alerts",
}

func gaps(u map[string]string) []string {
	out := []string{}
	for k, why := range u {
		name := sourceNames[k]
		if name == "" {
			name = k
		}
		out = append(out, name+": "+why)
	}
	sort.Strings(out)
	return out
}

func languages(m map[string]int) []Language {
	total := 0
	for _, b := range m {
		total += b
	}
	out := []Language{}
	for n, b := range m {
		out = append(out, Language{Name: n, Bytes: b, Share: math.Round(float64(b)/float64(max(total, 1))*1000) / 1000})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Bytes > out[j].Bytes })
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}

// ---- commits ---------------------------------------------------------------

// Classify returns "bot", "ai" (authored or co-authored by an AI assistant)
// or "human" for a commit.
func Classify(c gh.Commit) string {
	login, typ := "", ""
	if c.Author != nil {
		login = strings.ToLower(c.Author.Login)
		typ = c.Author.Type
	}
	name := strings.ToLower(c.Commit.Author.Name)
	email := strings.ToLower(c.Commit.Author.Email)
	msg := strings.ToLower(c.Commit.Message)
	switch {
	case typ == "Bot", strings.HasSuffix(login, "[bot]"), strings.Contains(name, "[bot]"), strings.HasPrefix(name, "dependabot"):
		return "bot"
	case name == "claude", login == "claude", strings.Contains(email, "@anthropic.com"),
		strings.Contains(msg, "co-authored-by: claude"), strings.Contains(msg, "generated with [claude code]"),
		strings.Contains(msg, "co-authored-by: copilot"), strings.Contains(name, "copilot"):
		return "ai"
	}
	return "human"
}

func authorName(c gh.Commit) string {
	if c.Author != nil && c.Author.Login != "" {
		return c.Author.Login
	}
	if c.Commit.Author.Name != "" {
		return c.Commit.Author.Name
	}
	return "unknown"
}

func activity(d *gh.RepoData, now time.Time) Activity {
	a := Activity{Truncated: len(d.Commits) >= commitCap, Trend: "flat"}
	thisWeek := weekStart(now)
	idx := map[string]int{}
	for i := weeksShown - 1; i >= 0; i-- {
		ws := thisWeek.AddDate(0, 0, -7*i).Format("2006-01-02")
		idx[ws] = len(a.Weekly)
		a.Weekly = append(a.Weekly, Week{Start: ws})
	}
	days30 := map[string]bool{}
	people := map[string]*Contributor{}
	for _, c := range d.Commits {
		t := c.Commit.Author.Date.UTC()
		kind := Classify(c)
		a.CommitsTotal++
		age := now.Sub(t)
		if age <= 7*24*time.Hour {
			a.Commits7d++
		}
		if age <= 30*24*time.Hour {
			a.Commits30d++
			days30[t.Format("2006-01-02")] = true
		}
		a.ByWeekday[(int(t.Weekday())+6)%7]++
		switch kind {
		case "bot":
			a.Authorship.Bot++
		case "ai":
			a.Authorship.AI++
		default:
			a.Authorship.Human++
		}
		if i, ok := idx[weekStart(t).Format("2006-01-02")]; ok {
			w := &a.Weekly[i]
			w.Total++
			switch kind {
			case "bot":
				w.Bot++
			case "ai":
				w.AI++
			default:
				w.Human++
			}
		}
		// Credit the human author too when a commit is AI co-authored, but
		// list the assistant separately when it is the author.
		name := authorName(c)
		ckind := kind
		if kind == "ai" && strings.ToLower(c.Commit.Author.Name) != "claude" && (c.Author == nil || strings.ToLower(c.Author.Login) != "claude") {
			ckind = "human"
		}
		p := people[name]
		if p == nil {
			p = &Contributor{Name: name, Kind: ckind}
			people[name] = p
		}
		p.Commits++
		if t.After(p.LastAt) {
			p.LastAt = t
		}
	}
	a.ActiveDays30d = len(days30)
	if a.CommitsTotal > 0 {
		a.Authorship.AIShare = math.Round(float64(a.Authorship.AI)/float64(a.CommitsTotal)*1000) / 1000
		a.Authorship.BotShare = math.Round(float64(a.Authorship.Bot)/float64(a.CommitsTotal)*1000) / 1000
	}
	for _, p := range people {
		a.Contributors = append(a.Contributors, *p)
		if p.Kind == "human" {
			a.HumanAuthors++
		}
	}
	sort.Slice(a.Contributors, func(i, j int) bool { return a.Contributors[i].Commits > a.Contributors[j].Commits })
	if a.Contributors == nil {
		a.Contributors = []Contributor{}
	}

	// Trend: last 4 complete-or-current weeks vs the 4 before.
	n := len(a.Weekly)
	recent, prior := 0, 0
	for i := n - 4; i < n; i++ {
		recent += a.Weekly[i].Total
	}
	for i := n - 8; i < n-4; i++ {
		prior += a.Weekly[i].Total
	}
	switch {
	case prior == 0 && recent > 0:
		a.Trend = "up"
	case prior > 0:
		pct := int(math.Round(float64(recent-prior) / float64(prior) * 100))
		a.TrendPct = &pct
		if pct >= 20 {
			a.Trend = "up"
		} else if pct <= -20 {
			a.Trend = "down"
		}
	}
	return a
}

func fillWeeklyFlow(a *Activity, d *gh.RepoData) {
	idx := map[string]int{}
	for i, w := range a.Weekly {
		idx[w.Start] = i
	}
	for _, p := range d.ClosedPulls {
		if p.MergedAt != nil {
			if i, ok := idx[weekStart(*p.MergedAt).Format("2006-01-02")]; ok {
				a.Weekly[i].Merged++
			}
		}
	}
	for _, is := range d.ClosedIssue {
		if i, ok := idx[weekStart(*is.ClosedAt).Format("2006-01-02")]; ok {
			a.Weekly[i].Closed++
		}
	}
}

func status(d *gh.RepoData, now time.Time) Status {
	s := Status{LastPushAt: d.Repo.PushedAt, RepoCreatedAt: d.Repo.CreatedAt, ProjectAgeDays: daysBetween(now, d.Repo.CreatedAt)}
	var ref time.Time
	if len(d.Commits) > 0 {
		c := d.Commits[0]
		t := c.Commit.Author.Date.UTC()
		s.LastCommitAt = &t
		s.LastCommitTitle = firstLine(c.Commit.Message)
		s.LastCommitBy = authorName(c)
		s.LastCommitURL = c.HTMLURL
		ref = t
	} else {
		ref = d.Repo.PushedAt
	}
	days := daysBetween(now, ref)
	s.DaysSinceCommit = intp(days)
	switch {
	case d.Repo.Archived:
		s.State, s.Label = "archived", "Archived on GitHub"
	case d.Repo.Size == 0 && len(d.Commits) == 0:
		s.State, s.Label = "empty", "Empty repository"
	case days <= 7:
		s.State, s.Label = "active", "Active"
	case days <= 30:
		s.State, s.Label = "slowing", fmt.Sprintf("Slowing: no commits for %d days", days)
	default:
		s.State, s.Label = "stalled", fmt.Sprintf("Stalled: no commits for %d days", days)
	}
	return s
}

// ---- pull requests ---------------------------------------------------------

func brief(p gh.Pull, now time.Time) PRBrief {
	return PRBrief{Number: p.Number, Title: firstLine(p.Title), Author: p.User.Login, URL: p.HTMLURL,
		AgeDays: daysBetween(now, p.CreatedAt), IdleDays: daysBetween(now, p.UpdatedAt), Draft: p.Draft}
}

func pulls(d *gh.RepoData, now time.Time) Pulls {
	out := Pulls{Stale: []PRBrief{}, OpenList: []PRBrief{}, RecentMerged: []PRBrief{}}
	open := append([]gh.Pull(nil), d.OpenPulls...)
	sort.Slice(open, func(i, j int) bool { return open[i].CreatedAt.Before(open[j].CreatedAt) })
	for _, p := range open {
		out.Open++
		if p.Draft {
			out.OpenDraft++
		}
		b := brief(p, now)
		if b.AgeDays > out.OldestOpenDays {
			out.OldestOpenDays = b.AgeDays
		}
		if b.IdleDays >= staleAfterDays && !p.Draft {
			out.Stale = append(out.Stale, b)
		}
		if len(out.OpenList) < 15 {
			out.OpenList = append(out.OpenList, b)
		}
	}
	var hours []float64
	merged := []gh.Pull{}
	for _, p := range d.ClosedPulls {
		if p.ClosedAt == nil || now.Sub(*p.ClosedAt) > 30*24*time.Hour {
			if p.MergedAt != nil && now.Sub(*p.MergedAt) <= 14*24*time.Hour {
				merged = append(merged, p)
			}
			continue
		}
		if p.MergedAt != nil {
			out.Merged30d++
			hours = append(hours, p.MergedAt.Sub(p.CreatedAt).Hours())
			if now.Sub(*p.MergedAt) <= 14*24*time.Hour {
				merged = append(merged, p)
			}
		} else {
			out.ClosedUnmerged30++
		}
	}
	out.MedianMergeHours = percentile(hours, 0.5)
	out.P90MergeHours = percentile(hours, 0.9)
	sort.Slice(merged, func(i, j int) bool { return merged[i].MergedAt.After(*merged[j].MergedAt) })
	for _, p := range merged {
		if len(out.RecentMerged) >= 15 {
			break
		}
		b := brief(p, now)
		t := p.MergedAt.UTC()
		b.At = &t
		out.RecentMerged = append(out.RecentMerged, b)
	}
	return out
}

// ---- CI --------------------------------------------------------------------

func counted(conclusion string) (ok, counts bool) {
	switch conclusion {
	case "success":
		return true, true
	case "failure", "timed_out", "startup_failure":
		return false, true
	}
	return false, false // cancelled, skipped, neutral, action_required, ""
}

func ci(d *gh.RepoData, lookback int, now time.Time) CI {
	out := CI{Known: d.Unavailable["actions"] == "", DefaultStatus: "none", Workflows: []WorkflowCI{}, Failing: []string{}}
	if !out.Known {
		out.DefaultStatus = "unknown"
		return out
	}
	runs := append([]gh.WorkflowRun(nil), d.Runs...)
	sort.Slice(runs, func(i, j int) bool { return runs[i].CreatedAt.After(runs[j].CreatedAt) })
	out.Runs = len(runs)
	out.RunsPerWeek = math.Round(float64(len(runs))/(float64(lookback)/7)*10) / 10

	var okAll, nAll, okDef, nDef int
	var mins []float64
	type wf struct {
		WorkflowCI
		ok, n       int
		defaultSet  bool
		defaultLast string
	}
	byName := map[string]*wf{}
	order := []string{}
	for _, r := range runs {
		w := byName[r.Name]
		if w == nil {
			w = &wf{WorkflowCI: WorkflowCI{Name: r.Name}}
			byName[r.Name] = w
			order = append(order, r.Name)
		}
		w.Runs++
		if r.Status != "completed" {
			continue
		}
		if w.Last == "" {
			w.Last, w.LastAt, w.URL = r.Conclusion, r.UpdatedAt, r.HTMLURL
		}
		if !r.RunStartedAt.IsZero() && r.UpdatedAt.After(r.RunStartedAt) {
			mins = append(mins, r.UpdatedAt.Sub(r.RunStartedAt).Minutes())
		}
		isDefault := r.HeadBranch == d.Repo.DefaultBranch && r.Event != "pull_request"
		good, counts := counted(r.Conclusion)
		if counts {
			nAll++
			w.n++
			if good {
				okAll++
				w.ok++
			}
			if isDefault {
				nDef++
				if good {
					okDef++
				}
				if !w.defaultSet {
					w.defaultSet, w.defaultLast = true, r.Conclusion
				}
			}
		}
		if isDefault && out.LastDefaultRun == nil && counts {
			out.LastDefaultRun = &RunBrief{Workflow: r.Name, Conclusion: r.Conclusion, At: r.UpdatedAt, URL: r.HTMLURL}
		}
	}
	out.SuccessRate = ratio(okAll, nAll)
	out.DefaultSuccessRate = ratio(okDef, nDef)
	out.MedianMinutes = percentile(mins, 0.5)
	anyDefault := false
	for _, name := range order {
		w := byName[name]
		w.SuccessRate = ratio(w.ok, w.n)
		out.Workflows = append(out.Workflows, w.WorkflowCI)
		if w.defaultSet {
			anyDefault = true
			if w.defaultLast != "success" {
				out.Failing = append(out.Failing, name)
			}
		}
	}
	if anyDefault {
		out.DefaultStatus = "passing"
		if len(out.Failing) > 0 {
			out.DefaultStatus = "failing"
		}
	}
	return out
}

// ---- releases --------------------------------------------------------------

func releases(d *gh.RepoData, lookback int, now time.Time) Releases {
	out := Releases{Known: d.Unavailable["releases"] == ""}
	var latest *gh.Release
	for i := range d.Releases {
		r := &d.Releases[i]
		if r.Draft || r.PublishedAt.IsZero() {
			continue
		}
		if daysBetween(now, r.PublishedAt) <= lookback {
			out.InLookback++
		}
		if latest == nil || r.PublishedAt.After(latest.PublishedAt) {
			latest = r
		}
	}
	if latest != nil {
		t := latest.PublishedAt.UTC()
		out.LatestTag, out.LatestAt, out.URL = latest.TagName, &t, latest.HTMLURL
		out.DaysSince = intp(daysBetween(now, t))
	}
	return out
}

// ---- issues ----------------------------------------------------------------

func issues(d *gh.RepoData, now time.Time) Issues {
	out := Issues{Known: d.Unavailable["issues_open"] == "", Aging: []Bucket{}, ByLabel: []LabelCount{}, OldestList: []IssueBrief{}}
	buckets := []Bucket{{Label: "under 7 days"}, {Label: "7–30 days"}, {Label: "30–90 days"}, {Label: "over 90 days"}}
	labels := map[string]int{}
	var ages []float64
	open := append([]gh.Issue(nil), d.OpenIssues...)
	sort.Slice(open, func(i, j int) bool { return open[i].CreatedAt.Before(open[j].CreatedAt) })
	for _, is := range open {
		out.Open++
		age := daysBetween(now, is.CreatedAt)
		ages = append(ages, float64(age))
		if age > out.OldestOpenDays {
			out.OldestOpenDays = age
		}
		if age <= 30 {
			out.Opened30d++
		}
		if hasLabel(is.Labels, "bug") {
			out.BugsOpen++
		}
		if len(is.Labels) == 0 {
			out.Unlabeled++
		}
		if len(is.Assignees) == 0 {
			out.Unassigned++
		}
		for _, l := range is.Labels {
			labels[l.Name]++
		}
		switch {
		case age < 7:
			buckets[0].Count++
		case age < 30:
			buckets[1].Count++
		case age < 90:
			buckets[2].Count++
		default:
			buckets[3].Count++
		}
		if len(out.OldestList) < 5 {
			out.OldestList = append(out.OldestList, IssueBrief{Number: is.Number, Title: firstLine(is.Title), URL: is.HTMLURL, AgeDays: age})
		}
	}
	var closeDays []float64
	for _, is := range d.ClosedIssue {
		if now.Sub(*is.ClosedAt) <= 30*24*time.Hour {
			out.Closed30d++
			closeDays = append(closeDays, is.ClosedAt.Sub(is.CreatedAt).Hours()/24)
		}
		if daysBetween(now, is.CreatedAt) <= 30 {
			out.Opened30d++
		}
	}
	out.Net30d = out.Opened30d - out.Closed30d
	out.MedianOpenAge = percentile(ages, 0.5)
	out.MedianCloseDays = percentile(closeDays, 0.5)
	out.Aging = buckets
	for l, n := range labels {
		out.ByLabel = append(out.ByLabel, LabelCount{Label: l, Count: n})
	}
	sort.Slice(out.ByLabel, func(i, j int) bool {
		if out.ByLabel[i].Count != out.ByLabel[j].Count {
			return out.ByLabel[i].Count > out.ByLabel[j].Count
		}
		return out.ByLabel[i].Label < out.ByLabel[j].Label
	})
	if len(out.ByLabel) > 8 {
		out.ByLabel = out.ByLabel[:8]
	}
	return out
}

// ---- progress --------------------------------------------------------------

func progress(d *gh.RepoData, now time.Time) Progress {
	out := Progress{Milestones: []MilestoneProgress{}}
	closedBy := map[string]int{}
	for _, is := range d.ClosedIssue {
		if is.Milestone != nil && now.Sub(*is.ClosedAt) <= 30*24*time.Hour {
			closedBy[is.Milestone.Title]++
		}
	}
	var sumOpen, sumClosed int
	for _, m := range d.Milestones {
		mp := MilestoneProgress{Title: m.Title, State: m.State, Open: m.OpenIssues, Closed: m.ClosedIssues, URL: m.HTMLURL}
		if tot := m.OpenIssues + m.ClosedIssues; tot > 0 {
			mp.Percent = math.Round(float64(m.ClosedIssues)/float64(tot)*1000) / 10
		}
		if m.DueOn != nil {
			t := m.DueOn.UTC()
			mp.DueOn = &t
			dt := int(math.Floor(t.Sub(now).Hours() / 24))
			mp.DaysToDue = &dt
			mp.Overdue = m.State == "open" && dt < 0 && m.OpenIssues > 0
		}
		if m.State == "open" {
			sumOpen += m.OpenIssues
			sumClosed += m.ClosedIssues
			if rate := float64(closedBy[m.Title]) / 30; rate > 0 && m.OpenIssues > 0 {
				f := int(math.Ceil(float64(m.OpenIssues) / rate))
				mp.ForecastDays = &f
				if mp.DaysToDue != nil {
					on := f <= *mp.DaysToDue
					mp.OnTrack = &on
				}
			} else if m.OpenIssues > 0 && mp.DaysToDue != nil {
				on := false
				mp.OnTrack = &on
			}
		}
		out.Milestones = append(out.Milestones, mp)
	}
	sort.SliceStable(out.Milestones, func(i, j int) bool {
		a, b := out.Milestones[i], out.Milestones[j]
		if a.State != b.State {
			return a.State == "open"
		}
		if a.DueOn != nil && b.DueOn != nil {
			return a.DueOn.Before(*b.DueOn)
		}
		return a.DueOn != nil
	})
	if len(out.Milestones) > 10 {
		out.Milestones = out.Milestones[:10]
	}
	for i := range out.Milestones {
		m := &out.Milestones[i]
		if m.State == "open" {
			if out.Current == nil || (m.DueOn != nil && out.Current.DueOn == nil) {
				cp := *m
				out.Current = &cp
			}
		}
	}
	out.OpenMilestonePct = nil
	if sumOpen+sumClosed > 0 {
		v := math.Round(float64(sumClosed)/float64(sumOpen+sumClosed)*1000) / 10
		out.OpenMilestonePct = &v
	}
	done := 0
	for _, p := range d.ClosedPulls {
		if p.MergedAt != nil && now.Sub(*p.MergedAt) <= 28*24*time.Hour {
			done++
		}
	}
	for _, is := range d.ClosedIssue {
		if now.Sub(*is.ClosedAt) <= 28*24*time.Hour {
			done++
		}
	}
	out.ThroughputWeek = math.Round(float64(done)/4*10) / 10
	if len(d.Milestones) == 0 {
		out.Note = "No milestones yet. Create GitHub milestones (e.g. 'MVP', 'v1.0') and attach issues to them to get % complete, due dates and finish forecasts. Until then progress is measured by throughput."
	}
	return out
}

// ---- hygiene & security ----------------------------------------------------

func hygiene(d *gh.RepoData) Hygiene {
	h := Hygiene{Known: d.Community != nil, Branches: len(d.Branches), SizeKB: d.Repo.Size}
	if d.Community != nil {
		f := d.Community.Files
		h.CommunityHealth = d.Community.HealthPercentage
		h.Readme, h.License, h.Contributing = f.Readme != nil, f.License != nil, f.Contributing != nil
		h.IssueTemplate, h.PRTemplate = f.IssueTemplate != nil, f.PullRequestTemplate != nil
	}
	for _, b := range d.Branches {
		if b.Name == d.Repo.DefaultBranch {
			p := b.Protected
			h.DefaultProtected = &p
		}
	}
	return h
}

func sevRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium", "moderate":
		return 2
	}
	return 3
}

func addSev(s *Severities, sev string) {
	s.Total++
	switch sevRank(sev) {
	case 0:
		s.Critical++
	case 1:
		s.High++
	case 2:
		s.Medium++
	default:
		s.Low++
	}
}

func security(d *gh.RepoData) Security {
	s := Security{
		DependabotKnown: d.Unavailable["dependabot"] == "",
		CodeScanKnown:   d.Unavailable["code_scanning"] == "",
		SecretsKnown:    d.Unavailable["secret_scanning"] == "",
		Top:             []AlertBrief{},
	}
	for _, a := range d.Dependabot {
		sev := strings.ToLower(a.SecurityAdvisory.Severity)
		addSev(&s.Dependabot, sev)
		s.Top = append(s.Top, AlertBrief{Source: "dependabot", Severity: sev, URL: a.HTMLURL,
			Title: firstLine(a.Dependency.Package.Name + ": " + a.SecurityAdvisory.Summary)})
	}
	for _, a := range d.CodeScan {
		sev := strings.ToLower(a.Rule.SecuritySeverityLevel)
		if sev == "" {
			sev = map[string]string{"error": "high", "warning": "medium"}[a.Rule.Severity]
		}
		addSev(&s.CodeScanning, sev)
		s.Top = append(s.Top, AlertBrief{Source: "code scanning", Severity: sev, URL: a.HTMLURL, Title: firstLine(a.Rule.Description)})
	}
	for _, a := range d.Secrets {
		s.SecretsOpen++
		s.Top = append(s.Top, AlertBrief{Source: "secret scanning", Severity: "critical", URL: a.HTMLURL, Title: "Exposed secret: " + a.SecretTypeDisplayName})
	}
	sort.SliceStable(s.Top, func(i, j int) bool { return sevRank(s.Top[i].Severity) < sevRank(s.Top[j].Severity) })
	if len(s.Top) > 10 {
		s.Top = s.Top[:10]
	}
	return s
}
