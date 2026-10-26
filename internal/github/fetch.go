package github

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"sync"
	"time"
)

// RepoData is everything Project Pulse collects for one repository.
type RepoData struct {
	Repo        Repo
	Languages   map[string]int
	Commits     []Commit // default branch, since the lookback start
	OpenPulls   []Pull
	ClosedPulls []Pull // closed or merged since the lookback start
	OpenIssues  []Issue
	ClosedIssue []Issue // closed since the lookback start
	Milestones  []Milestone
	Runs        []WorkflowRun // since the lookback start
	Releases    []Release
	Branches    []Branch
	Community   *Community
	Dependabot  []DependabotAlert
	CodeScan    []CodeScanningAlert
	Secrets     []SecretScanningAlert

	// Unavailable lists optional sources the token couldn't read, keyed by
	// source name, with the reason. Analytics treat them as unknown, not zero.
	Unavailable map[string]string
}

// FetchRepo collects RepoData for fullName ("owner/repo"). Only the repo
// itself is required; every other source degrades gracefully.
func (c *Client) FetchRepo(ctx context.Context, fullName string, since time.Time) (*RepoData, error) {
	base := "/repos/" + fullName
	d := &RepoData{Unavailable: map[string]string{}}
	if err := c.Get(ctx, base, nil, &d.Repo); err != nil {
		return nil, err
	}
	sinceISO := since.UTC().Format(time.RFC3339)

	var mu sync.Mutex
	var wg sync.WaitGroup
	var hardErr error
	run := func(name string, f func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := f(); err != nil {
				mu.Lock()
				defer mu.Unlock()
				if IsPermission(err) {
					d.Unavailable[name] = "not visible to this token (missing permission or feature turned off)"
					return
				}
				d.Unavailable[name] = err.Error()
				if hardErr == nil && ctx.Err() != nil {
					hardErr = ctx.Err()
				}
			}
		}()
	}

	run("languages", func() error { return c.Get(ctx, base+"/languages", nil, &d.Languages) })
	run("commits", func() (err error) {
		q := url.Values{"since": {sinceISO}, "sha": {d.Repo.DefaultBranch}}
		d.Commits, err = List[Commit](ctx, c, base+"/commits", q, 10)
		return
	})
	run("pulls_open", func() (err error) {
		d.OpenPulls, err = List[Pull](ctx, c, base+"/pulls", url.Values{"state": {"open"}}, 5)
		return
	})
	run("pulls_closed", func() error {
		q := url.Values{"state": {"closed"}, "sort": {"updated"}, "direction": {"desc"}}
		ps, err := List[Pull](ctx, c, base+"/pulls", q, 5)
		for _, p := range ps {
			if p.ClosedAt != nil && !p.ClosedAt.Before(since) {
				d.ClosedPulls = append(d.ClosedPulls, p)
			}
		}
		return err
	})
	run("issues_open", func() error {
		is, err := List[Issue](ctx, c, base+"/issues", url.Values{"state": {"open"}}, 10)
		for _, i := range is {
			if i.PullRequest == nil {
				d.OpenIssues = append(d.OpenIssues, i)
			}
		}
		return err
	})
	run("issues_closed", func() error {
		q := url.Values{"state": {"closed"}, "since": {sinceISO}}
		is, err := List[Issue](ctx, c, base+"/issues", q, 10)
		for _, i := range is {
			if i.PullRequest == nil && i.ClosedAt != nil && !i.ClosedAt.Before(since) {
				d.ClosedIssue = append(d.ClosedIssue, i)
			}
		}
		return err
	})
	run("milestones", func() (err error) {
		d.Milestones, err = List[Milestone](ctx, c, base+"/milestones", url.Values{"state": {"all"}}, 2)
		return
	})
	run("actions", func() error {
		q := url.Values{"created": {">=" + since.UTC().Format("2006-01-02")}, "per_page": {"100"}}
		next := c.url(base+"/actions/runs", q)
		for page := 0; next != "" && page < 5; page++ {
			var wr WorkflowRuns
			n, err := c.do(ctx, next, base+"/actions/runs", &wr)
			if err != nil {
				return err
			}
			d.Runs = append(d.Runs, wr.WorkflowRuns...)
			next = n
		}
		return nil
	})
	run("releases", func() (err error) {
		d.Releases, err = List[Release](ctx, c, base+"/releases", nil, 1)
		return
	})
	run("branches", func() (err error) {
		d.Branches, err = List[Branch](ctx, c, base+"/branches", nil, 3)
		return
	})
	run("community", func() error {
		var cp Community
		if err := c.Get(ctx, base+"/community/profile", nil, &cp); err != nil {
			return err
		}
		d.Community = &cp
		return nil
	})
	run("dependabot", func() (err error) {
		d.Dependabot, err = List[DependabotAlert](ctx, c, base+"/dependabot/alerts", url.Values{"state": {"open"}}, 3)
		return
	})
	run("code_scanning", func() (err error) {
		d.CodeScan, err = List[CodeScanningAlert](ctx, c, base+"/code-scanning/alerts", url.Values{"state": {"open"}}, 3)
		return
	})
	run("secret_scanning", func() (err error) {
		d.Secrets, err = List[SecretScanningAlert](ctx, c, base+"/secret-scanning/alerts", url.Values{"state": {"open"}}, 3)
		return
	})
	wg.Wait()
	if hardErr != nil {
		return nil, fmt.Errorf("fetch %s: %w", fullName, hardErr)
	}
	sort.Slice(d.Commits, func(i, j int) bool { return d.Commits[i].Commit.Author.Date.After(d.Commits[j].Commit.Author.Date) })
	return d, nil
}
