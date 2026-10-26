# Project Pulse

Live status, progress and risk analytics for your main GitHub projects, built for a tech lead or project manager doing a weekly check-in. It runs on your own machine in Docker, reads GitHub with your token, and serves:

- a **dashboard** at `http://localhost:8080`: portfolio overview plus a page per project
- a **JSON API** under `/api/...` for scripts, other tools or your own UIs
- a **markdown status report** at `/api/report.md` to paste into Slack, email or a doc

Nothing is exposed publicly: Docker binds the port to `127.0.0.1` only.

## Quick start

```bash
git clone https://github.com/KING-CYBERTON/project-pulse && cd project-pulse
cp .env.example .env        # then paste your token into GITHUB_TOKEN
make up                     # or: docker compose up -d --build
# open http://localhost:8080
make down                   # stop when you're done
```

The first load takes a few seconds while it reads every repo; after that results are cached for 10 minutes. Press **Refresh from GitHub** to fetch fresh data.

### Create a token

Use a **fine-grained personal access token** (GitHub → Settings → Developer settings → Fine-grained tokens → Generate new token):

- **Resource owner:** KING-CYBERTON
- **Repository access:** Only select repositories → pick the repos in `projects.json`
- **Repository permissions** (all **Read-only**):
  - Metadata (added automatically), Contents, Issues, Pull requests, Actions
  - Dependabot alerts, Code scanning alerts, Secret scanning alerts (for the security section)
  - Administration (only if you want branch-protection status)

Anything the token can't read shows up as "not visible to this token" and is left out of the score, never counted as zero.

## Which projects are tracked

Edit `projects.json`, then `make restart` (no rebuild needed):

```json
{
  "lookback_days": 90,
  "projects": [
    { "name": "ayopa", "repo": "KING-CYBERTON/ayopa", "kind": "product", "description": "…", "owner": "Kim" }
  ]
}
```

- `name`: short id used in URLs (lowercase, digits, `-`, `_`)
- `kind`: `product` gets full scoring; `profile` (e.g. your profile README) is scored on freshness and README only
- `owner` (optional): who's accountable, shown in the API

## What it measures

| Area | Metrics |
|---|---|
| **Status** | Active (commit in last 7 days), slowing (8–30), stalled (30+), archived, empty; last commit, author and message; project age |
| **Health score** | 0–100, grade A–F, RAG (green ≥70, amber ≥50, red). Weighted factors: activity 20, CI 20, pull requests 15, backlog 15, security 15, docs & setup 10, delivery 5. A failing default branch or an exposed secret is never green. Each factor shows its note so the score is explainable. |
| **Activity** | Commits 7d / 30d / lookback, active days, 4-week trend vs the previous 4, weekly series (12 weeks), weekday pattern, contributors, **human vs AI-assisted vs bot** authorship |
| **Pull requests** | Open / draft, stale (no update in 7+ days), merged and abandoned in 30 days, median and p90 time to merge, recently merged list |
| **CI** | Default branch passing/failing and which workflows fail, pass rate overall and on the default branch, runs per week, median duration, per-workflow table |
| **Releases** | Latest tag, days since, releases in the lookback window |
| **Backlog** | Open issues, bugs, opened vs closed in 30 days (net growth), age buckets, median time to close, top labels, unlabelled / unassigned, oldest issues |
| **Progress** | Milestone % complete, due dates, overdue, **forecast days to finish** from the last 30 days' closing rate, on-track flag, weekly throughput (merged PRs + closed issues) |
| **Security & setup** | Open Dependabot, code-scanning and secret-scanning alerts by severity; README, license, contributing guide, templates, branch protection, community health % |
| **Risks** | Prioritised list (high / medium / low) with a reason and a link: failing builds, exposed secrets, critical alerts, stalled work, stale PRs, overdue or at-risk milestones, growing backlog, bus factor of 1, heavy AI authorship without review, unprotected default branch, missing README, data the token can't see |

> **Tip:** progress forecasting needs GitHub **milestones**. Create one per release (e.g. "MVP", "v1.0"), set a due date, and attach issues to it.

## API

All endpoints are `GET` unless noted and return JSON.

| Endpoint | Returns |
|---|---|
| `/api/health` | Service status, GitHub rate limit, last refresh |
| `/api/portfolio` | Totals, one summary row per project (worst health first), weekly commits by project, where the work went (30 days), cross-project "needs attention" list |
| `/api/projects` | Summary rows only |
| `/api/projects/{name}` | Full report for one project |
| `/api/projects/{name}/{section}` | One section: `status`, `health`, `risks`, `activity`, `pulls`, `ci`, `releases`, `issues`, `progress`, `hygiene`, `security` |
| `/api/risks?severity=high` | Risks across all projects, optionally filtered |
| `/api/report.md?days=7` | Markdown status report; "shipped" covers the last `days` (1–30) |
| `POST /api/refresh` | Re-fetch everything from GitHub now |

Examples:

```bash
curl -s localhost:8080/api/portfolio | jq '.projects[] | {name, rag, score, state}'
curl -s localhost:8080/api/projects/ayopa/ci | jq
curl -s localhost:8080/api/report.md > status.md
```

## Run without Docker

Needs Go 1.24+. No third-party dependencies.

```bash
make run      # loads .env, serves on :8080
make test
```

## Limits

- Commits are counted on each repo's **default branch**; work on unmerged branches shows up through pull requests instead. At most 1,000 commits per lookback window are read.
- GitHub's community-profile endpoint (README/license check) only works for public repos; for private repos that section is reported as unknown.
- The token's rate limit is 5,000 requests an hour; one refresh of five projects uses roughly 75–100.
