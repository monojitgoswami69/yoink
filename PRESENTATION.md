# Yoink — Selection Deck Content

> Source content for the 4-slide pre-hackathon submission. Third person
> throughout. Each section maps 1:1 to one slide of the supplied template.

---

## Slide 1 — Problem Statement, Understanding & Approach

### Problem Statement

Docker has become the de-facto standard for running, shipping, and sharing
modern software. Yet the path from "clone a repository" to "the application
runs on my machine" remains one of the most painful, high-friction
experiences in software development — especially for newcomers.

A developer cloning an unfamiliar project today faces a stack of obstacles:

- **No Dockerfile, or an outdated one** — the developer must write or fix
  it without knowing the framework's build conventions.
- **Multi-service repositories** require a `docker-compose.yml` that
  orchestrates frontends, backends, and workers in concert.
- **Infrastructure dependencies** (PostgreSQL, Redis, MongoDB) are
  referenced in source code but never declared in compose, causing silent
  runtime failures.
- **Environment variables** are scattered across the codebase with no
  consolidated `.env` template.
- **Build failures** demand Docker fluency to interpret and resolve.
- **Existing tools generate scaffolding and walk away** — none verify the
  result actually runs.

### Understanding

The barrier is not a single missing file. It is the **entire chain** from
repository to running stack. Tools like `docker init`, Nixpacks, and Cloud
Native Buildpacks each solve one slice of this chain, but they all share
the same blind spot: they generate artefacts and assume the user will
debug what doesn't work. For a Docker beginner, that assumption is the
problem.

### Approach

**Yoink** is a command-line tool that takes a GitHub URL and produces a
**verified, running** Docker environment — not just a Dockerfile.

Three principles shape the approach:

1. **Static first, LLM second.** Deterministic detection handles the
   common case; an LLM safety net handles the long tail of failures.
2. **Generate and verify, not just generate.** Yoink runs what it
   produces, observes failures, and self-corrects via a sandboxed
   LLM-driven heal loop.
3. **Stay in the terminal.** A TUI dashboard delivers daily operation
   (env editing, restarts, logs, resource view) without a separate web
   UI or external dependency.

---

## Slide 2 — Detailed Proposal & Solution Approach

### The Proposal

A single command — `yoink init <github-url>` — that takes a repository
from a URL to a healthy, addressable Docker stack on the developer's
machine, with no Docker expertise required on the user's part.

### End-to-End Pipeline

1. **Clone** the repository (supports HTTPS, SSH, branch-specific, and
   sub-directory URLs; private repos via PAT injected securely through
   git's `http.extraheader`, never the command line).
2. **Detect services** with framework-level granularity. Identifies
   Next.js, NestJS, Fastify, Express, Vite, CRA, plain React, FastAPI,
   Flask, Django, generic Node, and generic Python. Monorepo-aware:
   recognises multiple deployables in `apps/*` and `services/*`.
3. **Analyse source code** beyond surface scanning: reads entry files to
   learn runtime ports, extracts environment-variable references for the
   `.env` template, picks the right package manager from lockfiles.
4. **Infer infrastructure**: when source code references `DATABASE_URL`,
   `REDIS_URL`, `MONGO_URI`, etc., Yoink appends the matching service
   (postgres, redis, mongo) to compose with sensible defaults and
   connection strings.
5. **Generate** one framework-specific Dockerfile per service plus a
   single `docker-compose.yml` with healthchecks, restart policies, and
   conflict-free host port bindings.
6. **Build and verify** the stack against the host's Docker daemon.
7. **Self-heal on failure**: build errors are routed to the LLM with the
   failing Dockerfile and the relevant source code. The LLM proposes a
   correction; Yoink re-tries the build. Bounded retries; clean
   fallback to a human-readable error summary if convergence fails.
8. **Hand off** to a Bubble Tea TUI dashboard for ongoing operation.

### Key Differentiators

- **Ships software, not files.** Existing tools generate scaffolding.
  Yoink generates *and* runs *and* heals.
- **Framework-level templates**, not language-level. A Next.js app, a
  Vite SPA, and an Express API get genuinely different Dockerfiles.
- **Infrastructure inference** from source code — no manual compose
  edits to add a database.
- **LLM-driven heal loop with sandboxed file access** — security by
  construction; the model cannot read outside the repo root.
- **Stateful**: env values, port choices, and last-known configuration
  persist under `~/.yoink/state/<repo>/`, so re-runs are zero-input.
- **Provider-agnostic**: OpenAI, Anthropic, Groq, Google Gemini, and
  local Ollama all supported; users pick at setup.
- **Fully terminal-native**: zero browser, zero extra processes, single
  static binary.

### Target Users

- Developers new to Docker who want their app running, not a Docker
  certification.
- Experienced developers who clone unfamiliar repositories for
  evaluation, contribution, or review.
- Educators and bootcamps who need a frictionless way to demo
  containerisation.

---

## Slide 3 — System Architecture

### Architectural Diagram (recreate in template)

```
                       ┌──────────────────────┐
                       │   GitHub URL Input   │
                       └──────────┬───────────┘
                                  ▼
                  ┌───────────────────────────────┐
                  │   CLI Layer  (Cobra-based)    │
                  └───────────────┬───────────────┘
                                  ▼
                  ┌───────────────────────────────┐
                  │     Pipeline Orchestrator     │
                  └─┬───────┬───────┬───────┬─────┘
                    ▼       ▼       ▼       ▼
       ┌────────────┐  ┌─────────┐ ┌──────┐ ┌──────────┐
       │  Git       │  │Detection│ │Infra │ │Generation│
       │  Cloner    │  │ Engine  │ │Inferr│ │  Engine  │
       │ (PAT-safe) │  │(JS/Py)  │ │ -er  │ │          │
       └────────────┘  └─────────┘ └──────┘ └─────┬────┘
                                                  ▼
                                        ┌─────────────────┐
                                        │  Dockerfiles +  │
                                        │  compose.yml +  │
                                        │  .env templates │
                                        └────────┬────────┘
                                                 ▼
                                       ┌──────────────────┐
                                       │ Execution Engine │
                                       │ (docker compose) │
                                       └────────┬─────────┘
                                                ▼
                                          ┌──────────┐
                                          │ Success? │
                                          └────┬─────┘
                                          no   │   yes
                                ┌──────────────┘   └──────────┐
                                ▼                             ▼
                  ┌──────────────────────────┐    ┌──────────────────────┐
                  │  Heal Loop               │    │   Bubble Tea TUI     │
                  │  ────────────            │    │   Dashboard          │
                  │  • LLM client (5 prov.)  │    │  • Service status    │
                  │  • Sandboxed file reader │    │  • Env editor        │
                  │  • Patched Dockerfile    │    │  • Log tail          │
                  │  • Retry (bounded)       │    │  • Resource monitor  │
                  └────────────┬─────────────┘    └──────────┬───────────┘
                               │                             │
                               └──────────► retry ◄──────────┘
                                                              │
                                                              ▼
                                                  ┌──────────────────────┐
                                                  │  Persistence Layer   │
                                                  │  ~/.yoink/state/   │
                                                  │  • env-overrides     │
                                                  │  • settings          │
                                                  │  • detection lock    │
                                                  └──────────────────────┘
```

### Module Responsibilities

| Module | Responsibility |
|---|---|
| **CLI Layer** | Argument parsing, flags, command dispatch, global UX (colours, quiet mode). |
| **Pipeline Orchestrator** | Drives the eight-step pipeline; threads context through; coordinates spinners and step output. |
| **Git Cloner** | URL parsing for HTTPS/SSH/branch/subdir; secure PAT injection via `http.extraheader`; never persists secrets to disk. |
| **Detection Engine** | Framework detection (JS/TS + Python), monorepo walking, package-manager inference, source-level port discovery, confidence scoring. |
| **Infrastructure Inferrer** | Pattern-matches infrastructure references in env/code; appends compose services for postgres/redis/mongo/etc. |
| **Generation Engine** | Per-framework Dockerfile templates; compose builder with healthchecks, restart policies, port-conflict resolution; `.env.example` synthesis. |
| **Execution Engine** | Driver around the host's `docker` CLI; captures structured build output for the heal loop. |
| **Heal Loop** | Sends failures to the LLM with sandboxed file access; applies returned corrections; bounded retries; clean fallback on non-convergence. |
| **Sandboxed File Reader** | Path-cleaned, symlink-safe, size-capped reader rooted at the cloned repo; rejects absolute paths and `..` traversal. |
| **LLM Provider Client** | Provider-agnostic interface over OpenAI, Anthropic, Groq, Gemini, Ollama; robust JSON extraction tolerant of markdown fences and prose. |
| **TUI Dashboard** | Bubble Tea + Lipgloss surfaces: service list, inline env editor (hot-restart on save), log tail, resource view. |
| **Persistence Layer** | JSON-on-disk under `~/.yoink/state/<repo>/`; remembers env overrides, settings, and detection state across runs. |

### Security & Safety Properties

- LLM file access is sandboxed: cannot escape the cloned repo root,
  cannot read absolute paths, cannot follow symlinks outward, hard
  size cap per file.
- API keys and GitHub PATs are stored in `~/.yoink/config.json` with
  `0600` permissions; never logged, never embedded in URLs.
- Retry budget is bounded; on non-convergence the user receives a
  human-readable failure summary rather than an infinite loop.

---

## Slide 4 — Technologies Involved / Used

### Core Stack

| Layer | Technology | Rationale |
|---|---|---|
| **Language** | Go (1.21+) | Single static binary, fast startup, native concurrency, no runtime dependency on the user's machine. |
| **CLI Framework** | Cobra | Industry-standard Go CLI library; handles flags, subcommands, help generation. |
| **TUI Framework** | Bubble Tea | Elm-style reactive TUI used by Charm; powers the dashboard, env editor, and selection wizard. |
| **Styling** | Lipgloss | Composable terminal styling with NO_COLOR support for accessibility. |
| **Container Runtime** | Host Docker + Docker Compose v2 | Uses the user's native installation; no daemon, no shim, no extra binary. |
| **Source Repository** | Git CLI (subprocess) | Reliable across platforms; PAT injected via `http.extraheader` for security. |

### LLM Integration

| Provider | API Style | Use Case |
|---|---|---|
| **OpenAI** | Chat Completions | Default cloud option. |
| **Anthropic Claude** | Messages API | Strong reasoning for heal loop. |
| **Groq** | OpenAI-compatible | Fast inference for low-latency demos. |
| **Google Gemini** | GenerateContent API | Cost-efficient cloud option. |
| **Ollama** | Local generate API | Fully offline mode; no API key required. |

All providers expose the same internal interface; provider choice is
made at setup time and persists in config.

### Supporting Libraries & Patterns

- **Standard library**: `encoding/json`, `regexp`, `net/http`,
  `os/exec`, `path/filepath`, `io/fs` — the bulk of the work uses no
  third-party dependencies for portability and audit-ability.
- **golang.org/x/term**: masked password / API key input.
- **gopkg.in/yaml.v3**: compose-file validation in tests.

### Supported Frameworks at Launch

| Ecosystem | Frameworks |
|---|---|
| **JavaScript / TypeScript** | Next.js, NestJS, Fastify, Express, Koa, Hapi, Vite, Create-React-App, plain React, generic Node. |
| **Python** | FastAPI, Django, Flask, generic Python. |
| **Inferred Infrastructure** | PostgreSQL, Redis, MongoDB, RabbitMQ, Elasticsearch. |

### Design Constraints Honoured

- **Single binary, no runtime dependencies** beyond `git` and the user's
  existing Docker installation.
- **Cross-platform**: Linux, macOS, Windows (where Docker Desktop is
  available).
- **NO_COLOR convention** respected for accessibility and CI use.
- **Offline-capable** via Ollama or `--no-agent` flag (pure static
  analysis, no LLM round-trips).
