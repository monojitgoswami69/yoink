# Yoink Landing Page — Context Analysis & Content Blueprint

> Pre-design/pre-development planning document. Everything below is sourced
> from the actual repository: `README.md`, `PRESENTATION.md`, `ROADMAP.md`,
> `cmd/*.go`, and `install.sh`. Sections marked **GAP** need assets or
> decisions before build starts.

---

## Part 1 — Context Analysis

### 1.1 Product Snapshot

| Attribute | Value (verified in repo) |
|---|---|
| What it is | Go CLI that turns a GitHub URL into a **verified, running** Docker environment |
| One-line essence | `yoink init <github-url>` → clone → detect → infer infra → generate → build → **self-heal** → run |
| Pipeline | 8 steps, surfaced as `[1/8] … ✓` progress lines (real sample completes in ~38s) |
| Binary profile | Single static Go binary; deps at runtime are only `git` + existing Docker/Compose v2 |
| Platforms | Linux, macOS, Windows (Docker Desktop) |
| Command surface | 19 commands (`init, setup, list, status, up, down, restart, open, logs, env, stats, config, heal, update, dash, remove, doctor, explain, help`) + aliases `start`/`stop`; global flags `-v/-q/--no-color/--version` |
| Detection scope | 14 framework profiles: Next.js, NestJS, Fastify, Express, Koa, Hapi, Vite, CRA, plain React, generic Node, FastAPI, Django, Flask, generic Python |
| Infra inference | Postgres, MySQL, Redis, MongoDB, RabbitMQ, Elasticsearch — inferred from env-var refs **and** dependency manifests |
| LLM layer | Provider-agnostic: OpenAI, Anthropic, Groq, Gemini, Ollama (local); bounded heal loop (default 3 tries); `--no-agent` pure-static mode |
| Safety posture | Sandboxed file reader (symlink/traversal-proof, 16 KiB caps); PAT via `http.extraheader` never argv/config; `~/.yoink/config.json` chmod 0600; masked secret input |
| Statefulness | `~/.yoink/state/<repo>/` (`yoink.lock`, `env-overrides.json`, `settings.json`) → zero-input re-runs |
| License | MIT |

### 1.2 Core Value Proposition (ranked by persuasive strength)

1. **Ships software, not files.** Competitors generate scaffolding and walk
   away; Yoink runs what it produces, observes failures, and repairs them.
2. **Zero Docker expertise required.** The whole chain from URL → healthy,
   addressable localhost stack happens in one command.
3. **Framework-level intelligence**, not language-level: a Next.js app, a
   Vite SPA, and an Express API get genuinely different Dockerfiles.
4. **Infrastructure inference**: reads your source, sees `DATABASE_URL`,
   adds Postgres with healthchecks and wired connection strings.
5. **It remembers you**: persistent state makes the second `yoink up`
   require no input at all.
6. **Terminal-native + private**: no browser, no daemon, sandboxed LLM,
   works offline (Ollama / `--no-agent`).

### 1.3 Audience Segments → Page Jobs

| Segment | Job to be done | Page section that converts them |
|---|---|---|
| Docker newcomers | "Just make it run" | Hero + How-it-works + Install |
| Experienced devs cloning unfamiliar repos | Evaluate/contribute fast | Feature bento + detection matrix + `explain` command |
| Educators / bootcamps | Frictionless demos | Use-case strip + FAQ |
| Security-conscious engineers | Will the LLM read my secrets? | Security & Privacy section |
| Open-source evaluators | Is this alive and trustworthy? | GitHub stars, MIT badge, roadmap link, doctor command |

### 1.4 Competitive Frame (for comparison section)

| Capability | `docker init` | Nixpacks / Buildpacks | Hand-written compose | Yoink |
|---|---|---|---|---|
| Generates Dockerfile | ✔ | ✔ | manual | ✔ framework-level |
| Multi-service monorepo | ✖ | partial | manual | ✔ `apps/*`, `services/*` |
| Infers DB/cache from source | ✖ | ✖ | manual | ✔ 6 services |
| Builds & verifies result | ✖ | builds | manual | ✔ |
| Auto-repairs broken builds | ✖ | ✖ | manual | ✔ bounded heal loop |
| Day-2 ops (logs/env/stats/TUI) | ✖ | ✖ | compose CLI | ✔ `dash`, `logs`, `env`, `stats` |
| Works offline | ✔ | ✔ | ✔ | ✔ Ollama / `--no-agent` |

### 1.5 Voice & Tone

- Third-person product voice already established in PRESENTATION.md; reuse it.
- Confident, concrete, numbers-forward ("14 frameworks · 6 backing services ·
  5 LLM providers · 1 binary").
- Show real terminal output everywhere; never invent output.
- Respect the tool's own values on the page: dark-first, `NO_COLOR`
  accessibility ethos, keyboard-friendly.

---

## Part 2 — Conversion Model

**Primary action:** copy the install one-liner.
**Secondary actions:** GitHub star, view README/docs, watch demo.

Success metrics: `install_copy_click`, `github_outbound_ctr`, scroll-depth to
install section, demo-replay rate, FAQ engagement.

Every major section ends in a micro-CTA back to install or GitHub.

---

## Part 3 — Page Architecture (top → bottom)

### S0. Sticky Nav
Logo · How it works · Features · Install · Commands · FAQ · GitHub (star
count badge) · **CTA button: "Get started"** → scrolls to install.
Mobile: hamburger; keep install CTA always visible.

### S1. Hero — the 5-second pitch
- **H1 candidates** (A/B test):
  1. "From GitHub URL to running Docker stack. One command."
  2. "Give Yoink a repository. Get a running application."
  3. "Your app, containerized, provisioned, healed, and running — before your coffee cools."
- Subhead: *"Yoink clones any repo, detects its services, provisions the
  infrastructure it needs, generates Docker configs, repairs broken builds
  with an LLM heal loop, and hands you a running stack — no Docker expertise required."*
- **Centerpiece: animated terminal** replaying the *real* `[1/8]…[8/8]` init
  session verbatim from README (typing animation, respects
  `prefers-reduced-motion` with static fallback).
- Primary CTA: install one-liner with copy button:
  `curl -fsSL https://raw.githubusercontent.com/monojitgoswami69/yoink/main/install.sh | bash`
- Secondary CTAs: "Star on GitHub" · "See how it works ↓"
- Micro-trust row: `MIT` · `Linux / macOS / Windows` · `Single binary` · `Works offline`

### S2. Proof Strip
Framework marquee/logos of the 14 detected frameworks + 6 infra services
(Next.js, FastAPI, Django, Redis, Postgres…) — "Yoink speaks your stack."
**GAP:** no star-count history/testimonials yet; substitute compatibility
marquee until community proof exists.

### S3. Problem → Solution ("The chain, not a file")
- Left: the six failure points from PRESENTATION.md (outdated Dockerfile,
  multi-service compose, undeclared infra, scattered env vars, cryptic build
  failures, tools that walk away).
- Right: how Yoink closes each gap. Framing line: *"The barrier isn't a
  missing Dockerfile. It's everything between `git clone` and a healthy stack."*

### S4. How It Works — pipeline visualization
Horizontal/vertical stepper of the 8 stages (Clone → Tree → Detect → Env vars
→ Infer infra → Generate → Write → **Build & Heal**), each expandable with a
one-liner + tiny code/output snippet pulled from README. Highlight step 8
visually — the heal loop is the differentiator; animate a failing-build →
LLM-patch → passing-build loop.

### S5. Feature Bento Grid (6 cards, each with micro-demo)
1. **Self-healing builds** — bounded retries, human-readable failure summary; show `yoink heal --tries 5 myrepo`.
2. **Infrastructure inference** — hint→service table condensed (`DATABASE_URL → postgres:16-alpine :5432`).
3. **Framework-aware generation** — multi-stage Next/Nest, nginx SPAs, slim Python.
4. **Monorepo-native** — compose context at repo root, `COPY apps/web/package*.json ./`.
5. **Persistent state** — tree of `~/.yoink/state/<repo>/`; "second `up` needs zero input".
6. **Any LLM or none** — 5-provider grid + Ollama offline + `--no-agent`.

### S6. TUI Dashboard Showcase
Full-width mock/screenshot of the Bubble Tea dashboard (service list, tabbed
Logs/Env/Controls panes, `docker stats` strip) + keybinding chips
(`↑/↓ select · tab pane · e edit env · r restart · q quit`). Copy angle:
*"Day-2 operations without leaving the terminal."*
**GAP:** need real captures/screen-recording.

### S7. Installation (conversion-critical — make it honest and multi-path)
Tabbed by platform (macOS/Linux | Windows):
1. One-liner script (current `install.sh` behavior; note `YOINK_INSTALL_DIR` override).
2. From source: `git clone && go build -o yoink .`.
3. Prereqs callout: Go 1.21+ to **build**, `git`; Docker Engine + Compose v2
   only at **runtime** (`--no-build` works without Docker).
- Post-install next-steps block mirroring install.sh output:
  `yoink setup` → `yoink init <url>` → `yoink up` → `yoink dash`.
- **GAP / decision:** no brew tap or GitHub release binaries exist yet. Adding
  `brew install` / `go install` / prebuilt binaries would materially lift
  conversion; until then the page must not imply otherwise.

### S8. Quickstart (60-second path)
Three annotated commands with expected output snippets; link the sample repo
(`tiangolo/full-stack-fastapi-template` used throughout README).
**GAP:** ROADMAP flags demo-repo selection as an open question — pick 2–3
canonical demo repos and record their real outputs for the page.

### S9. Command Reference
Searchable/filterable table of all 19 commands grouped exactly like
`cmd/help.go`: PROJECTS / RUNTIME / CONFIGURATION / INTELLIGENCE / UI & SYSTEM,
with args column and global flags panel. Each row deep-links to anchor;
client-side search input ("type to filter"). Include `explain` and `doctor`
prominently — they're trust builders.

### S10. Setup & Providers
`yoink setup` wizard walkthrough, config JSON sample, 5-provider grid
(Ollama card highlighted: "fully local, no API key"), PAT handling explainer
(`http.extraheader`, never argv, stripped post-clone), masked-input note.

### S11. Security & Privacy
Four pillars from PRESENTATION.md: sandboxed reader (path-traversal/symlink/
size-cap), 0600 config perms, bounded retries (no runaway loops), offline
mode. Tone: factual bullets, no fear-marketing.

### S12. Comparison Table
Use the §1.4 matrix. Fair wording; footnote what competitors *do* well.

### S13. Who It's For
Three persona cards (newcomer / reviewer / educator) with a one-sentence
scenario + the exact command sequence each would run.

### S14. FAQ (accordion)
Seed questions: Do I need an API key? (No — `--no-agent`/Ollama.) Which
languages? (JS/TS + Python today; roadmap.) Private repos? (PAT flow.) Does
it touch my Docker daemon? (Uses your existing installation.) What if the
heal loop fails? (Capped tries + readable summary.) Windows? (Docker Desktop.)
Where does state live? Is my code sent anywhere? (Only failing Dockerfile +
requested files ≤16 KiB through sandbox.)

### S15. Final CTA Band
Repeat one-liner + GitHub button + "Read the README/docs".

### S16. Footer
MIT license · repo link · ROADMAP link · version (from `cmd.Version`) ·
built-with note (Go, Cobra, Bubble Tea — developer credibility) ·
NO_COLOR/accessibility nod.

---

## Part 4 — Content Sourcing Map (keep page synced with repo)

| Page section | Source of truth |
|---|---|
| Hero terminal | README top block (verbatim) |
| Pipeline steps | PRESENTATION.md §End-to-End Pipeline + README |
| Feature claims | README "What Yoink does"; verify against `cmd/*.go` |
| Commands table | `cmd/help.go` groups (authoritative ordering) |
| Init flags | README (`--name --no-agent --force --output --no-build --heal-tries --max-services`) |
| Infra table | README infrastructure-inference table |
| Security | PRESENTATION.md §Security & Safety Properties + `internal/safefs` |
| Providers | README §Setup |
| Roadmap mentions | Link to `ROADMAP.md`, don't inline (avoids drift) |

Rule: any claim not backed by README/code gets cut or marked "planned → link
to ROADMAP".

---

## Part 5 — Design Direction

- Dark terminal aesthetic default (light mode optional); accent color from
  the Lipgloss palette used in `internal/ui` for brand coherence.
- Monospace display font for H1/commands (e.g. JetBrains Mono), clean sans
  for body (Inter).
- Real ANSI-colored terminal renders — no fake screenshots.
- Motion: typewriter hero (once, skippable), scroll-triggered pipeline
  highlight, subtle bento hover demos; all gated behind
  `prefers-reduced-motion`.
- Copy buttons on **every** code block with ✓ feedback.
- Accessibility mirrors the product: AA contrast, full keyboard nav,
  semantic headings.

---

## Part 6 — Technical Plan

- **Stack recommendation:** static site generator (Astro preferred; Next
  static export acceptable) deployed to GitHub Pages/Cloudflare Pages;
  zero-JS baseline with islands for: terminal animation, tabs, command
  filter, FAQ accordion, copy buttons.
- Budget: <150 KB JS, LCP <1.8 s, Lighthouse ≥95 across the board.
- Components inventory: `<Terminal/>`, `<PipelineSteps/>`, `<BentoCard/>`,
  `<InstallTabs/>`, `<CommandTable/>` (+search), `<ProviderGrid/>`,
  `<ComparisonTable/>`, `<FAQ/>`, `<CopyButton/>`, `<Kbd/>`, `<GitHubStars/>`.
- SEO: title *"Yoink — Turn any GitHub repo into a running Docker stack"*,
  meta description from README tagline, canonical, OG/Twitter card image
  (**GAP:** needs creation, 1200×630), JSON-LD `SoftwareApplication`
  (license MIT, platforms, free), sitemap.
- Analytics: event set from §2; cookieless.

---

## Part 7 — Asset & Decision Checklist Before Build

| # | Item | Status |
|---|---|---|
| 1 | Recorded asciinema/GIF of full init incl. heal-loop recovery | GAP — record |
| 2 | TUI dashboard screenshots/video | GAP — capture |
| 3 | Logo / favicon / wordmark | GAP — none in repo |
| 4 | OG share image | GAP — create |
| 5 | Canonical demo repos + their real outputs | OPEN QUESTION in ROADMAP |
| 6 | Release binaries / brew tap decision (affects install honesty) | DECISION needed |
| 7 | Star-count widget vs static badge | trivial |
| 8 | Domain (yoink.dev / getyoink.io…) | DECISION needed |
| 9 | Where site lives (`frontend/` dir exists, empty) | confirm |

---

## Part 8 — Build Phasing

- **Phase 0 (content):** lock copy bank from §3 sources; record assets #1–4.
- **Phase 1 MVP:** Nav, Hero(+terminal), Problem, Pipeline, Bento, Install,
  Quickstart, Commands table, Footer, minimal FAQ. Ship.
- **Phase 2:** Dashboard showcase, Security, Comparison, Personas, full FAQ,
  analytics, OG/SEO polish.
- **Phase 3:** `/docs` hub generated from README (Starlight), changelog feed,
  A/B headline test, community section once stars/issues justify proof.

---

*Prepared as the contract between analysis and implementation: design picks
visual language within Part 5; development implements Part 3 + Part 6; all
copy originates from Part 4 sources.*
