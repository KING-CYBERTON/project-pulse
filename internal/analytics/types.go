// Package analytics turns raw GitHub data into project status, progress,
// delivery and risk metrics for tech leads and project managers.
package analytics

import "time"

// Report is the full analysis of one project.
type Report struct {
	Name        string     `json:"name"`
	Repo        string     `json:"repo"`
	Kind        string     `json:"kind"`
	Description string     `json:"description"`
	Owner       string     `json:"owner,omitempty"`
	URL         string     `json:"url"`
	Private     bool       `json:"private"`
	Branch      string     `json:"default_branch"`
	Languages   []Language `json:"languages"`
	GeneratedAt time.Time  `json:"generated_at"`
	Lookback    int        `json:"lookback_days"`

	Status   Status   `json:"status"`
	Health   Health   `json:"health"`
	Risks    []Risk   `json:"risks"`
	Activity Activity `json:"activity"`
	Pulls    Pulls    `json:"pull_requests"`
	CI       CI       `json:"ci"`
	Releases Releases `json:"releases"`
	Issues   Issues   `json:"issues"`
	Progress Progress `json:"progress"`
	Hygiene  Hygiene  `json:"hygiene"`
	Security Security `json:"security"`

	// DataGaps names sources the token couldn't read. Metrics that depend
	// on them are reported as unknown rather than zero.
	DataGaps []string `json:"data_gaps"`
	// Error is set when the project couldn't be fetched at all.
	Error string `json:"error,omitempty"`
}

type Language struct {
	Name  string  `json:"name"`
	Bytes int     `json:"bytes"`
	Share float64 `json:"share"`
}

// Status says whether work is happening.
type Status struct {
	State           string     `json:"state"` // active | slowing | stalled | archived | empty
	Label           string     `json:"label"`
	LastCommitAt    *time.Time `json:"last_commit_at"`
	LastCommitTitle string     `json:"last_commit_title"`
	LastCommitBy    string     `json:"last_commit_by"`
	LastCommitURL   string     `json:"last_commit_url"`
	DaysSinceCommit *int       `json:"days_since_last_commit"`
	LastPushAt      time.Time  `json:"last_push_at"`
	RepoCreatedAt   time.Time  `json:"repo_created_at"`
	ProjectAgeDays  int        `json:"project_age_days"`
}

// Health is a 0-100 score with the factors behind it.
type Health struct {
	Score   int      `json:"score"`
	Grade   string   `json:"grade"` // A-F
	RAG     string   `json:"rag"`   // green | amber | red
	Factors []Factor `json:"factors"`
}

type Factor struct {
	Name  string  `json:"name"`
	Score float64 `json:"score"`
	Max   float64 `json:"max"`
	Note  string  `json:"note"`
}

// Risk is something a lead should look at.
type Risk struct {
	Project  string `json:"project,omitempty"`
	Severity string `json:"severity"` // high | medium | low
	Area     string `json:"area"`     // delivery | quality | security | process | people | data
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	URL      string `json:"url,omitempty"`
}

// Activity describes commit flow on the default branch.
type Activity struct {
	CommitsTotal  int           `json:"commits_in_lookback"`
	Commits7d     int           `json:"commits_7d"`
	Commits30d    int           `json:"commits_30d"`
	ActiveDays30d int           `json:"active_days_30d"`
	Trend         string        `json:"trend"` // up | down | flat
	TrendPct      *int          `json:"trend_pct"`
	Weekly        []Week        `json:"weekly"`
	ByWeekday     [7]int        `json:"by_weekday"` // Mon..Sun
	Authorship    Authorship    `json:"authorship"`
	Contributors  []Contributor `json:"contributors"`
	HumanAuthors  int           `json:"human_authors"`
	Truncated     bool          `json:"truncated"`
}

type Week struct {
	Start  string `json:"week_start"` // Monday, YYYY-MM-DD
	Total  int    `json:"total"`
	Human  int    `json:"human"`
	AI     int    `json:"ai_assisted"`
	Bot    int    `json:"bot"`
	Merged int    `json:"prs_merged"`
	Closed int    `json:"issues_closed"`
}

type Authorship struct {
	Human    int     `json:"human"`
	AI       int     `json:"ai_assisted"`
	Bot      int     `json:"bot"`
	AIShare  float64 `json:"ai_share"`
	BotShare float64 `json:"bot_share"`
}

type Contributor struct {
	Name    string    `json:"name"`
	Kind    string    `json:"kind"` // human | ai | bot
	Commits int       `json:"commits"`
	LastAt  time.Time `json:"last_at"`
}

// Pulls is pull-request flow.
type Pulls struct {
	Open             int       `json:"open"`
	OpenDraft        int       `json:"open_draft"`
	Merged30d        int       `json:"merged_30d"`
	ClosedUnmerged30 int       `json:"closed_unmerged_30d"`
	MedianMergeHours *float64  `json:"median_hours_to_merge_30d"`
	P90MergeHours    *float64  `json:"p90_hours_to_merge_30d"`
	OldestOpenDays   int       `json:"oldest_open_days"`
	Stale            []PRBrief `json:"stale"` // open, no update in 7+ days
	OpenList         []PRBrief `json:"open_list"`
	RecentMerged     []PRBrief `json:"recent_merged"`
}

type PRBrief struct {
	Number   int        `json:"number"`
	Title    string     `json:"title"`
	Author   string     `json:"author"`
	URL      string     `json:"url"`
	AgeDays  int        `json:"age_days"`
	IdleDays int        `json:"idle_days"`
	Draft    bool       `json:"draft"`
	At       *time.Time `json:"merged_at,omitempty"`
}

// CI is GitHub Actions health.
type CI struct {
	Known              bool         `json:"known"`
	Runs               int          `json:"runs_in_lookback"`
	SuccessRate        *float64     `json:"success_rate"` // completed runs, all branches
	DefaultSuccessRate *float64     `json:"default_branch_success_rate"`
	DefaultStatus      string       `json:"default_branch_status"` // passing | failing | none
	LastDefaultRun     *RunBrief    `json:"last_default_run"`
	MedianMinutes      *float64     `json:"median_duration_minutes"`
	Workflows          []WorkflowCI `json:"workflows"`
	Failing            []string     `json:"failing_workflows"`
	RunsPerWeek        float64      `json:"runs_per_week"`
}

type RunBrief struct {
	Workflow   string    `json:"workflow"`
	Conclusion string    `json:"conclusion"`
	At         time.Time `json:"at"`
	URL        string    `json:"url"`
}

type WorkflowCI struct {
	Name        string    `json:"name"`
	Runs        int       `json:"runs"`
	SuccessRate *float64  `json:"success_rate"`
	Last        string    `json:"last_conclusion"`
	LastAt      time.Time `json:"last_at"`
	URL         string    `json:"url"`
}

// Releases is shipping cadence.
type Releases struct {
	Known      bool       `json:"known"`
	LatestTag  string     `json:"latest_tag"`
	LatestAt   *time.Time `json:"latest_at"`
	DaysSince  *int       `json:"days_since_latest"`
	InLookback int        `json:"count_in_lookback"`
	URL        string     `json:"url,omitempty"`
}

// Issues is backlog health.
type Issues struct {
	Known           bool         `json:"known"`
	Open            int          `json:"open"`
	Opened30d       int          `json:"opened_30d"`
	Closed30d       int          `json:"closed_30d"`
	Net30d          int          `json:"net_30d"` // opened - closed; positive = backlog growing
	BugsOpen        int          `json:"bugs_open"`
	Unlabeled       int          `json:"unlabeled_open"`
	Unassigned      int          `json:"unassigned_open"`
	OldestOpenDays  int          `json:"oldest_open_days"`
	MedianOpenAge   *float64     `json:"median_open_age_days"`
	MedianCloseDays *float64     `json:"median_days_to_close_30d"`
	Aging           []Bucket     `json:"aging"`
	ByLabel         []LabelCount `json:"by_label"`
	OldestList      []IssueBrief `json:"oldest"`
}

type Bucket struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

type LabelCount struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

type IssueBrief struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	AgeDays int    `json:"age_days"`
}

// Progress is milestone completion and throughput.
type Progress struct {
	Milestones       []MilestoneProgress `json:"milestones"`
	Current          *MilestoneProgress  `json:"current_milestone"`
	OpenMilestonePct *float64            `json:"open_milestones_percent"`
	ThroughputWeek   float64             `json:"throughput_per_week"` // merged PRs + closed issues, 4-week average
	Note             string              `json:"note,omitempty"`
}

type MilestoneProgress struct {
	Title        string     `json:"title"`
	State        string     `json:"state"`
	Open         int        `json:"open"`
	Closed       int        `json:"closed"`
	Percent      float64    `json:"percent"`
	DueOn        *time.Time `json:"due_on"`
	DaysToDue    *int       `json:"days_to_due"`
	Overdue      bool       `json:"overdue"`
	ForecastDays *int       `json:"forecast_days_to_finish"`
	OnTrack      *bool      `json:"on_track"`
	URL          string     `json:"url"`
}

// Hygiene is repo setup quality.
type Hygiene struct {
	Known            bool  `json:"known"`
	CommunityHealth  int   `json:"community_health_percent"`
	Readme           bool  `json:"readme"`
	License          bool  `json:"license"`
	Contributing     bool  `json:"contributing"`
	IssueTemplate    bool  `json:"issue_template"`
	PRTemplate       bool  `json:"pr_template"`
	Branches         int   `json:"branches"`
	DefaultProtected *bool `json:"default_branch_protected"`
	SizeKB           int   `json:"size_kb"`
}

// Security is open alert counts. Known is false when the token can't read alerts.
type Security struct {
	DependabotKnown bool         `json:"dependabot_known"`
	Dependabot      Severities   `json:"dependabot"`
	CodeScanKnown   bool         `json:"code_scanning_known"`
	CodeScanning    Severities   `json:"code_scanning"`
	SecretsKnown    bool         `json:"secret_scanning_known"`
	SecretsOpen     int          `json:"secret_scanning_open"`
	Top             []AlertBrief `json:"top_alerts"`
}

type Severities struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Total    int `json:"total"`
}

type AlertBrief struct {
	Source   string `json:"source"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	URL      string `json:"url"`
}

// Portfolio rolls every project up for a weekly review.
type Portfolio struct {
	GeneratedAt time.Time    `json:"generated_at"`
	Lookback    int          `json:"lookback_days"`
	Totals      Totals       `json:"totals"`
	Projects    []Summary    `json:"projects"`
	Attention   []Risk       `json:"attention"`
	Weekly      []PortWeek   `json:"weekly"`
	Focus       []FocusShare `json:"focus_30d"`
}

type Totals struct {
	Projects       int `json:"projects"`
	Green          int `json:"green"`
	Amber          int `json:"amber"`
	Red            int `json:"red"`
	Commits7d      int `json:"commits_7d"`
	Commits30d     int `json:"commits_30d"`
	OpenPRs        int `json:"open_prs"`
	StalePRs       int `json:"stale_prs"`
	Merged30d      int `json:"prs_merged_30d"`
	OpenIssues     int `json:"open_issues"`
	OpenBugs       int `json:"open_bugs"`
	FailingCI      int `json:"projects_with_failing_ci"`
	HighRisks      int `json:"high_risks"`
	SecurityAlerts int `json:"security_alerts"`
}

type Summary struct {
	Name         string             `json:"name"`
	Repo         string             `json:"repo"`
	Kind         string             `json:"kind"`
	Description  string             `json:"description"`
	URL          string             `json:"url"`
	Private      bool               `json:"private"`
	State        string             `json:"state"`
	StateLabel   string             `json:"state_label"`
	Score        int                `json:"score"`
	Grade        string             `json:"grade"`
	RAG          string             `json:"rag"`
	LastCommitAt *time.Time         `json:"last_commit_at"`
	Commits7d    int                `json:"commits_7d"`
	Commits30d   int                `json:"commits_30d"`
	Trend        string             `json:"trend"`
	Spark        []int              `json:"weekly_commits"`
	OpenPRs      int                `json:"open_prs"`
	StalePRs     int                `json:"stale_prs"`
	Merged30d    int                `json:"prs_merged_30d"`
	OpenIssues   int                `json:"open_issues"`
	CIStatus     string             `json:"ci_status"`
	CISuccess    *float64           `json:"ci_success_rate"`
	Milestone    *MilestoneProgress `json:"current_milestone"`
	TopRisk      *Risk              `json:"top_risk"`
	RiskCount    int                `json:"risk_count"`
	Error        string             `json:"error,omitempty"`
}

type PortWeek struct {
	Start     string         `json:"week_start"`
	Total     int            `json:"total"`
	ByProject map[string]int `json:"by_project"`
}

type FocusShare struct {
	Name    string  `json:"name"`
	Commits int     `json:"commits"`
	Share   float64 `json:"share"`
}
