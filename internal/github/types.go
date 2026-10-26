package github

import "time"

// Only the fields Project Pulse uses are declared.

type User struct {
	Login string `json:"login"`
	Type  string `json:"type"`
}

type Repo struct {
	Name            string    `json:"name"`
	FullName        string    `json:"full_name"`
	Description     string    `json:"description"`
	Private         bool      `json:"private"`
	Archived        bool      `json:"archived"`
	DefaultBranch   string    `json:"default_branch"`
	Language        string    `json:"language"`
	OpenIssuesCount int       `json:"open_issues_count"`
	Size            int       `json:"size"`
	HTMLURL         string    `json:"html_url"`
	Topics          []string  `json:"topics"`
	CreatedAt       time.Time `json:"created_at"`
	PushedAt        time.Time `json:"pushed_at"`
}

type CommitAuthor struct {
	Name  string    `json:"name"`
	Email string    `json:"email"`
	Date  time.Time `json:"date"`
}

type Commit struct {
	SHA     string `json:"sha"`
	HTMLURL string `json:"html_url"`
	Commit  struct {
		Message string       `json:"message"`
		Author  CommitAuthor `json:"author"`
	} `json:"commit"`
	Author *User `json:"author"`
}

type Label struct {
	Name string `json:"name"`
}

type Pull struct {
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	State     string     `json:"state"`
	Draft     bool       `json:"draft"`
	HTMLURL   string     `json:"html_url"`
	User      User       `json:"user"`
	Labels    []Label    `json:"labels"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ClosedAt  *time.Time `json:"closed_at"`
	MergedAt  *time.Time `json:"merged_at"`
	Head      struct {
		Ref string `json:"ref"`
	} `json:"head"`
	RequestedReviewers []User `json:"requested_reviewers"`
}

type Milestone struct {
	Number       int        `json:"number"`
	Title        string     `json:"title"`
	State        string     `json:"state"`
	OpenIssues   int        `json:"open_issues"`
	ClosedIssues int        `json:"closed_issues"`
	DueOn        *time.Time `json:"due_on"`
	HTMLURL      string     `json:"html_url"`
}

type Issue struct {
	Number      int        `json:"number"`
	Title       string     `json:"title"`
	State       string     `json:"state"`
	HTMLURL     string     `json:"html_url"`
	User        User       `json:"user"`
	Labels      []Label    `json:"labels"`
	Assignees   []User     `json:"assignees"`
	Milestone   *Milestone `json:"milestone"`
	Comments    int        `json:"comments"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at"`
	PullRequest *struct{}  `json:"pull_request"`
}

type WorkflowRun struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	HeadBranch   string    `json:"head_branch"`
	Event        string    `json:"event"`
	Status       string    `json:"status"`
	Conclusion   string    `json:"conclusion"`
	HTMLURL      string    `json:"html_url"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	RunStartedAt time.Time `json:"run_started_at"`
}

type WorkflowRuns struct {
	TotalCount   int           `json:"total_count"`
	WorkflowRuns []WorkflowRun `json:"workflow_runs"`
}

type Release struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
}

type Branch struct {
	Name      string `json:"name"`
	Protected bool   `json:"protected"`
}

type fileRef struct {
	HTMLURL string `json:"html_url"`
}

type Community struct {
	HealthPercentage int `json:"health_percentage"`
	Files            struct {
		Readme              *fileRef `json:"readme"`
		License             *fileRef `json:"license"`
		Contributing        *fileRef `json:"contributing"`
		CodeOfConduct       *fileRef `json:"code_of_conduct"`
		IssueTemplate       *fileRef `json:"issue_template"`
		PullRequestTemplate *fileRef `json:"pull_request_template"`
	} `json:"files"`
}

type DependabotAlert struct {
	Number           int    `json:"number"`
	State            string `json:"state"`
	HTMLURL          string `json:"html_url"`
	SecurityAdvisory struct {
		Severity string `json:"severity"`
		Summary  string `json:"summary"`
	} `json:"security_advisory"`
	Dependency struct {
		Package struct {
			Name      string `json:"name"`
			Ecosystem string `json:"ecosystem"`
		} `json:"package"`
	} `json:"dependency"`
}

type CodeScanningAlert struct {
	Number  int    `json:"number"`
	State   string `json:"state"`
	HTMLURL string `json:"html_url"`
	Rule    struct {
		Severity              string `json:"severity"`
		SecuritySeverityLevel string `json:"security_severity_level"`
		Description           string `json:"description"`
	} `json:"rule"`
}

type SecretScanningAlert struct {
	Number                int    `json:"number"`
	State                 string `json:"state"`
	HTMLURL               string `json:"html_url"`
	SecretTypeDisplayName string `json:"secret_type_display_name"`
}
