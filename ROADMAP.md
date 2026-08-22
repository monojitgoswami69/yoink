# Yoink — Roadmap

Yoink clones a GitHub repository, identifies the deployable services inside
it, and produces a runnable Docker environment. The hackathon goal is to take
the project from "scaffolds files" to "ships a working stack" by adding an
LLM-driven build/heal loop, automatic infrastructure inference, persistent
state, and a TUI dashboard for day-to-day use.

## Current state (shipped)

CLI surface

- `yoink init <github-url>` — clone, analyse, generate.
- `yoink setup` — interactive provider/model/PAT wizard, writes
  `~/.yoink/config.json` (chmod 0600).
- `yoink help` — formatted help screen.
- Global flags: `--verbose`, `--quiet`, `--no-color`, `--version`.

Detection (`internal/detector`)

- Walks the cloned tree, ignoring `node_modules`, `.git`, `.venv`, build
  artefacts, plus monorepo noise dirs (`examples`, `tests`, `docs`, etc.).
- JS/TS: picks up `package.json`, classifies as Next, NestJS, Fastify,
  Express/Koa/Hapi, Vite, CRA, plain React, or generic Node. Detects package
  manager from lockfiles.
- Python: picks up `requirements.txt`/`pyproject.toml`/`Pipfile`, classifies
  as FastAPI, Django, Flask, or generic Python. Prefers poetry when a
  `poetry.lock` is present.
- Skips workspace-root `package.json`s and libraries with no framework and no
  start script.
- Per-framework defaults for install/build/start commands and ports.
- Refines port from regex over JS entry files (`process.env.PORT || 4000`,
  `.listen(4000)`).
- Confidence rating (`high`/`medium`/`low`) per service, used to cap the
  result at `--max-services`.

Generation (`internal/generator`)

- One Dockerfile per service, framework-aware (multi-stage for Next/Nest,
  nginx for static SPAs, slim for Python).
- Single `docker-compose.yml` with build context at repo root so monorepos
  work; per-service `env_file`, `container_name`, `networks`.
- Naive host-port collision avoidance within the generated compose file.

Env var extraction (`internal/envvar`)

- Greps source for `process.env.X`, `import.meta.env.X`, `NEXT_PUBLIC_`,
  `VITE_`, `REACT_APP_`, `os.environ[...]`, `os.getenv(...)`, etc.
- Picks up existing `.env.example`/`.env.sample`/`.env.template` files.
- Generates a `.env.example` per service, with framework-specific common
  vars (e.g. `DATABASE_URL` for FastAPI, `DJANGO_SETTINGS_MODULE` for Django)
  merged with discovered names.

LLM layer (`internal/llm`)

- Providers: OpenAI, Anthropic, Groq, Gemini, Ollama.
- Three validation passes against the static output: detection,
  Dockerfile+compose, env vars.
- File-request loop: the model can ask for up to 5 files per round, capped
  at 16 KiB each, read through a sandboxed reader.
- Robust JSON extraction tolerant of markdown fences and prose.

Sandboxing (`internal/safefs`)

- Path-cleaned reader rooted at the cloned repo; rejects absolute paths,
  symlink escapes, and `..` traversal. 1 MiB read cap.

Config (`internal/config`)

- `~/.yoink/config.json` with 0600 perms.
- Legacy `.yoink.env` fallback retained for one upgrade cycle.

UI (`internal/ui`, `internal/termio`)

- Lipgloss-based palette, NO_COLOR support, styled boxes/lines.
- Goroutine-backed braille spinner.
- ANSI-aware table with confidence bar.
- Banner + per-command header.
- Bubbletea list selector for the setup wizard.
- Masked password input for API keys / PATs.

Git integration (`internal/git`)

- Parses HTTPS, HTTPS `/tree/<branch>/<subdir>`, and SSH GitHub URLs.
- Shallow clone with optional branch, PAT injected via `http.extraheader`
  (never on the command line or in `.git/config`).

Tests

- Detector: monorepo, dedup, exclusions, workspace skip, library skip,
  directory hints, port-from-entry.
- Envvar: NEXT_PUBLIC capture, Python patterns, deterministic ordering,
  common-var dedup.
- Generator: Dockerfile-per-service, compose context, host-port
  collisions, static-SPA port override, YAML validity, name sanitisation.
- Git: URL parsing.
- Safefs: containment, escape attempts, size cap.
- Config: round-trip, legacy fallback, validation, permissions.

## Locked-in decisions

- **Persistent state location**: `~/.yoink/state/<repo>/` with:
  - `env-overrides.json` — user-edited values, separate from the generated
    `.env.example` template.
  - `settings.json` — port preferences, last-used flags, dashboard prefs.
  - `yoink.lock` — hash of the detection result so re-runs can tell when
    fundamentals have changed.
- **Resource monitoring scope**: read-only display of `docker stats` in the
  TUI. No history, no anomaly detection, no alerting.
- **Out of scope** for the hackathon: web UI, runtime self-healing beyond
  compose `restart` policies, Grafana-style historical metrics, multi-host
  orchestration.

## Planned features (build order)

The order matters: if time runs out after feature 3, the demo still wins.

### 1. Build-heal loop

The headline feature. Differentiates Yoink from `docker init`, Nixpacks,
buildpacks — all of which generate and walk away.

- After generation, run `docker compose build` (and optionally `up -d`)
  with output captured.
- On non-zero exit, extract the failing service + last N lines of build
  output.
- Hand the failure, the current Dockerfile, and (when relevant) the
  generated compose to the LLM with a "fix this build" prompt. Use the
  existing file-request loop so the model can pull more code if needed.
- Apply the returned Dockerfile/compose, retry. Cap attempts (suggest 3).
- Surface a clean summary in the TUI: which service, which attempt, what
  changed, final status.
- Done when: a known-broken sample repo (one we'll prepare) goes from
  failing build to passing build without human intervention.

### 2. Infrastructure inference

Pairs with the build-heal loop. Without this, most "build OK but `up`
fails" cases are "the app needs a DB and there isn't one."

- Scan env-var references and `.env.example` content for infrastructure
  hints: `DATABASE_URL`, `POSTGRES_*`, `MYSQL_*`, `REDIS_URL`/`REDIS_HOST`,
  `MONGO_URI`/`MONGODB_URL`, `RABBITMQ_*`, `ELASTIC_*`.
- For each hint, append a compose service with sensible defaults:
  postgres:16-alpine, redis:7-alpine, mongo:7, rabbitmq:3-management, etc.
- Wire `depends_on` with `condition: service_healthy` once healthchecks
  exist (feature 3).
- Populate the generated `.env.example` with the inferred connection
  string (`postgresql://app:app@postgres:5432/app`).
- Done when: a repo that reads `DATABASE_URL` gets a postgres service
  added automatically and the app talks to it on `up`.

### 3. Compose healthchecks + port conflict polish

Small additions, big credibility bumps.

- Per-framework healthcheck templates:
  - Backends (Express/Fastify/Nest/FastAPI/Flask/Django): `curl -fsS
    http://localhost:PORT/health || exit 1` with reasonable
    interval/timeout/retries.
  - Static SPAs behind nginx: `wget -qO- http://localhost/ || exit 1`.
  - Inferred infra (postgres/redis/mongo): the upstream-recommended check.
- `restart: on-failure` policy on all app services.
- Strengthen host-port resolution: probe ports on the host (not just
  inside the generated compose) and pick the next free port. Make sure the
  URL summary at the end reflects the final binding.
- Done when: a port already bound by another process on the host triggers
  a remap rather than a runtime collision.

### 4. Persistent state + one-command start

The "it remembers me" UX win.

- Introduce `~/.yoink/state/<repo>/` with the three files listed above.
- New command: `yoink up [<repo>]`. With no arg, infers from cwd or the
  most recent init. Reads `env-overrides.json`, applies on top of
  `.env.example`, runs `docker compose up -d`, waits for healthchecks,
  prints the final URLs.
- On re-run after the upstream repo has changed: detect mismatch via
  `yoink.lock`, prompt the user to re-detect or stay on the saved
  configuration.
- Done when: a second `yoink up` on the same repo brings the stack back
  with the env values from last time, no further input required.

### 5. TUI dashboard

Daily-driver UI for the running stack.

- New command: `yoink dash`, or the screen `yoink up` drops into.
- Bubbletea, multi-pane:
  - Service list with status (up/down/starting/unhealthy/healthy) and
    bound URLs.
  - Per-service env editor that writes through to `env-overrides.json`
    and triggers a `docker compose up -d --force-recreate <service>` for
    just that service on save.
  - Per-service log tail (`docker logs -f`).
  - Per-service controls: start, stop, restart, rebuild.
- Done when: a user can change `DATABASE_URL`, save, and see the service
  restart with the new value — without touching a terminal command.

### 6. Resource monitoring (display only)

Bottom strip in the dashboard.

- Poll `docker stats --no-stream --format json` on a tick (suggest 2s).
- Show CPU%, mem MB / limit, net I/O per service. Plain numbers; a single
  sparkline column is nice-to-have, not required.
- Done when: numbers update live in the dashboard and reflect what
  `docker stats` shows in another terminal.

## Open questions

These need to be decided before the build week starts, not during it.

- **Demo repo(s)** — pick 2–3 candidates that:
  - Are recognisable (judges can grok the stack at a glance).
  - Are multi-service if possible (shows monorepo handling + infra
    inference together).
  - Don't already ship a working Dockerfile (otherwise the heal loop has
    nothing to do).
  - Have at least one realistic failure mode the LLM can plausibly fix.
- **Hackathon track / theme** — affects pitch and what to emphasise (AI
  agent track, dev tooling track, infra track).
- **Timeline and team size** — sets which features above are realistic.
- **Naming for the start command** — `yoink up` (compose-aligned, my
  preference) vs `yoink start` (matches the existing vocabulary).

## What stays out

Documented so we don't drift into it under pressure:

- Web dashboard. The TUI does the same job with less risk and a stronger
  CLI aesthetic.
- Runtime self-healing beyond `restart: on-failure`. Inferring root cause
  from runtime logs is hard; out of scope.
- Historical metrics / alerting / anomaly detection. Re-implementing
  cAdvisor + Grafana wastes time judges don't reward.
- Multi-host or Kubernetes deployment. Local Docker only.
- Languages beyond JS/TS and Python for the demo. Adding Go/Rust/Java is
  trivial code-wise but expands the surface area for the heal loop to
  cover.
