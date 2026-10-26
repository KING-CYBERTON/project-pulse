// Package server exposes Project Pulse over HTTP: a JSON API, a markdown
// status report and an embedded dashboard.
package server

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/KING-CYBERTON/project-pulse/internal/analytics"
	gh "github.com/KING-CYBERTON/project-pulse/internal/github"
)

//go:embed web
var webFS embed.FS

// Version is set at build time with -ldflags.
var Version = "dev"

// Server wires HTTP handlers to the service.
type Server struct {
	svc    *Service
	client *gh.Client
	mux    *http.ServeMux
}

// New builds the HTTP handler.
func New(svc *Service, client *gh.Client) *Server {
	s := &Server{svc: svc, client: client, mux: http.NewServeMux()}
	web, _ := fs.Sub(webFS, "web")
	s.mux.Handle("GET /", http.FileServerFS(web))
	s.mux.HandleFunc("GET /api/health", s.health)
	s.mux.HandleFunc("GET /api/portfolio", s.portfolio)
	s.mux.HandleFunc("GET /api/projects", s.projects)
	s.mux.HandleFunc("GET /api/projects/{name}", s.project)
	s.mux.HandleFunc("GET /api/projects/{name}/{section}", s.section)
	s.mux.HandleFunc("GET /api/risks", s.risks)
	s.mux.HandleFunc("GET /api/report.md", s.report)
	s.mux.HandleFunc("POST /api/refresh", s.refresh)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	sw := &statusWriter{ResponseWriter: w, code: 200}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	s.mux.ServeHTTP(sw, r)
	log.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.code, time.Since(start).Round(time.Millisecond))
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(c int) { w.code = c; w.ResponseWriter.WriteHeader(c) }

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// snapshot fetches with a context detached from the request, so a closed
// browser tab doesn't abort a refresh other requests are waiting on.
func (s *Server) snapshot(r *http.Request, force bool) (*Snapshot, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Minute)
	defer cancel()
	return s.svc.Snapshot(ctx, force)
}

func (s *Server) get(w http.ResponseWriter, r *http.Request) (*Snapshot, bool) {
	snap, err := s.snapshot(r, false)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "couldn't load data from GitHub: "+err.Error())
		return nil, false
	}
	return snap, true
}

func find(snap *Snapshot, name string) *analytics.Report {
	for i := range snap.Reports {
		if snap.Reports[i].Name == name {
			return &snap.Reports[i]
		}
	}
	return nil
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{
		"ok":                  true,
		"version":             Version,
		"projects_configured": len(s.svc.cfg.Projects),
		"token_configured":    s.svc.cfg.Token != "",
		"cache_ttl_seconds":   int(s.svc.cfg.CacheTTL.Seconds()),
		"lookback_days":       s.svc.cfg.LookbackDays,
		"github_rate_limit":   s.client.Rate(),
	}
	if snap := s.svc.Cached(); snap != nil {
		out["last_refresh"] = snap.At
		out["last_refresh_took_ms"] = snap.Took.Milliseconds()
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) portfolio(w http.ResponseWriter, r *http.Request) {
	if snap, ok := s.get(w, r); ok {
		writeJSON(w, http.StatusOK, snap.Portfolio)
	}
}

func (s *Server) projects(w http.ResponseWriter, r *http.Request) {
	if snap, ok := s.get(w, r); ok {
		writeJSON(w, http.StatusOK, snap.Portfolio.Projects)
	}
}

func (s *Server) project(w http.ResponseWriter, r *http.Request) {
	snap, ok := s.get(w, r)
	if !ok {
		return
	}
	rep := find(snap, r.PathValue("name"))
	if rep == nil {
		writeErr(w, http.StatusNotFound, "no project named "+strconv.Quote(r.PathValue("name"))+"; see /api/projects")
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (s *Server) section(w http.ResponseWriter, r *http.Request) {
	snap, ok := s.get(w, r)
	if !ok {
		return
	}
	rep := find(snap, r.PathValue("name"))
	if rep == nil {
		writeErr(w, http.StatusNotFound, "no project named "+strconv.Quote(r.PathValue("name")))
		return
	}
	sections := map[string]any{
		"status":   rep.Status,
		"health":   rep.Health,
		"risks":    rep.Risks,
		"activity": rep.Activity,
		"pulls":    rep.Pulls,
		"ci":       rep.CI,
		"releases": rep.Releases,
		"issues":   rep.Issues,
		"progress": rep.Progress,
		"hygiene":  rep.Hygiene,
		"security": rep.Security,
	}
	v, ok := sections[r.PathValue("section")]
	if !ok {
		writeErr(w, http.StatusNotFound, "unknown section; use one of status, health, risks, activity, pulls, ci, releases, issues, progress, hygiene, security")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) risks(w http.ResponseWriter, r *http.Request) {
	snap, ok := s.get(w, r)
	if !ok {
		return
	}
	sev := r.URL.Query().Get("severity")
	out := []analytics.Risk{}
	for _, rep := range snap.Reports {
		for _, k := range rep.Risks {
			if sev == "" || k.Severity == sev {
				out = append(out, k)
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) report(w http.ResponseWriter, r *http.Request) {
	snap, ok := s.get(w, r)
	if !ok {
		return
	}
	days := 7
	if v := r.URL.Query().Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 30 {
			writeErr(w, http.StatusBadRequest, "days must be a number from 1 to 30")
			return
		}
		days = n
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	_, _ = w.Write([]byte(Markdown(snap, days)))
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	snap, err := s.snapshot(r, true)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "refresh failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"refreshed_at": snap.At, "took_ms": snap.Took.Milliseconds(), "projects": len(snap.Reports)})
}
