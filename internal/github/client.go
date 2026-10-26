// Package github is a small, dependency-free client for the parts of the
// GitHub REST API that Project Pulse needs. It is read-only.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultBaseURL is the public GitHub API.
const DefaultBaseURL = "https://api.github.com"

// Client talks to the GitHub REST API with a personal access token.
type Client struct {
	base  string
	token string
	http  *http.Client

	mu   sync.Mutex
	rate RateLimit
}

// RateLimit is the most recent rate-limit state GitHub reported.
type RateLimit struct {
	Limit     int       `json:"limit"`
	Remaining int       `json:"remaining"`
	Reset     time.Time `json:"reset"`
}

// APIError is a non-2xx response from GitHub.
type APIError struct {
	Status  int
	Path    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("github %s: %d %s", e.Path, e.Status, e.Message)
}

// IsPermission reports whether err means the token can't see this resource
// (missing scope, feature disabled, or repo not visible).
func IsPermission(err error) bool {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae.Status == http.StatusForbidden || ae.Status == http.StatusNotFound || ae.Status == http.StatusUnauthorized
	}
	return false
}

// New returns a client. base may be empty for the public API.
func New(base, token string) *Client {
	if base == "" {
		base = DefaultBaseURL
	}
	return &Client{
		base:  strings.TrimRight(base, "/"),
		token: token,
		http:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Rate returns the last rate-limit state seen.
func (c *Client) Rate() RateLimit {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rate
}

var linkNext = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// do performs a GET on a full URL and decodes JSON into out. It returns the
// URL of the next page, if any.
func (c *Client) do(ctx context.Context, fullURL, path string, out any) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "project-pulse")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	c.recordRate(resp.Header)

	if resp.StatusCode == http.StatusAccepted {
		// GitHub is still computing (stats endpoints). Treat as empty.
		return "", nil
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var m struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &m)
		if m.Message == "" {
			m.Message = http.StatusText(resp.StatusCode)
		}
		return "", &APIError{Status: resp.StatusCode, Path: path, Message: m.Message}
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("decode %s: %w", path, err)
	}
	next := ""
	if m := linkNext.FindStringSubmatch(resp.Header.Get("Link")); m != nil {
		next = m[1]
	}
	return next, nil
}

func (c *Client) recordRate(h http.Header) {
	rem, err := strconv.Atoi(h.Get("X-RateLimit-Remaining"))
	if err != nil {
		return
	}
	lim, _ := strconv.Atoi(h.Get("X-RateLimit-Limit"))
	reset, _ := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64)
	c.mu.Lock()
	c.rate = RateLimit{Limit: lim, Remaining: rem, Reset: time.Unix(reset, 0).UTC()}
	c.mu.Unlock()
}

func (c *Client) url(path string, q url.Values) string {
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

// Get fetches a single JSON object.
func (c *Client) Get(ctx context.Context, path string, q url.Values, out any) error {
	_, err := c.do(ctx, c.url(path, q), path, out)
	return err
}

// List fetches up to maxPages pages of a list endpoint (100 items a page).
func List[T any](ctx context.Context, c *Client, path string, q url.Values, maxPages int) ([]T, error) {
	if q == nil {
		q = url.Values{}
	}
	q.Set("per_page", "100")
	next := c.url(path, q)
	var all []T
	for page := 0; next != "" && page < maxPages; page++ {
		var batch []T
		n, err := c.do(ctx, next, path, &batch)
		if err != nil {
			return all, err
		}
		all = append(all, batch...)
		next = n
	}
	return all, nil
}
