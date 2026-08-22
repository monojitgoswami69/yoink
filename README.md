# Yoink

> Give Yoink a repository. Yoink understands it, containerizes it,
> provisions the infrastructure it needs, repairs build failures, and gives
> you a runnable application — locally, with an LLM-driven build/heal loop.

```
$ yoink init https://github.com/tiangolo/full-stack-fastapi-template
[1/8] Clone repository                          ✓ cloned
[2/8] Generate repository tree                  ✓ Tree generated
[3/8] Detect services and frameworks            ✓ fastapi · next · react
[4/8] Extract environment variables             ✓ 4 services
[5/8] Infer backing services                    ✓ postgres (DATABASE_URL)
[6/8] Generate Docker configuration            ✓ Dockerfile + compose
[7/8] Write outputs                             ✓ .dockerignore + .env.example
[8/8] Build & heal                              ✓ Build succeeded
Init complete in 38.4s

  yoink env fastapi-template      configure the app
  yoink up fastapi-template       start it
  yoink status fastapi-template   is it healthy?
  yoink dash fastapi-template     open the dashboard
```

## What Yoink does

- **Detects** the deployable services in a repo (Node + Python frameworks).
- **Generates** per-service Dockerfiles + a single `docker-compose.yml`
  with healthchecks, restart policies, and host-port collision avoidance.
- **Infers** backing services from your env-var references (Postgres,
  MySQL, Redis, Mongo, RabbitMQ, Elasticsearch) and wires them into the
  compose with healthchecks, named volumes, and `depends_on:
  service_healthy`.
- **Heals** broken builds by asking the LLM to repair the failing
  Dockerfile or compose, then retrying — up to three attempts by
  default.
- **Persists** the result under `~/.yoink/state/<repo>/` so subsequent
  `yoink up` / `yoink dash` runs need no further input.
- **Drives** the running stack from a Bubbletea dashboard with live
  logs, env editing, per-service controls, and a `docker stats` strip.

## Install

```bash
git clone https://github.com/monojitgoswami69/yoink.git
cd yoink
go build -o yoink .
sudo mv yoink /usr/local/bin/           # or ~/.local/bin
# or run the bundled installer:
./install.sh
```

Requirements:

- Go 1.21+ to build
- `git`
- Docker Engine + Compose v2 (only at run time — `init` works without
  Docker if you pass `--no-build`)

## Commands

```
yoink setup                       Configure global provider, model, API key, GitHub PAT
yoink init <github-url> [--name] Clone, generate, run build/heal loop → a project
yoink list [--running|--stopped] List initialized projects
yoink status [project]           Show health and per-service status
yoink up   [project]             Start a project (waits for healthchecks)
yoink down [project]             Stop a project (preserves volumes)
yoink restart [project]          Restart a project
yoink open  [project] [--dash]   Open the app URL (or dashboard) in a browser
yoink logs  <project> [service]  Show logs (--follow to tail, --tail N)
yoink env   <project> [set|unset|list]  Manage app environment (masked)
yoink stats <project>            Show CPU / memory / net I/O per service
yoink config <project>           Per-project settings (heal tries, auto-heal, LLM)
yoink heal  [project] [--tries]  Re-run the LLM build/heal loop
yoink update [project]           Pull, regenerate, rebuild, restart
yoink dash  [project]            Live TUI dashboard (logs, env, controls, stats)
yoink remove <project> [--volumes] [--yes]  Remove a project (volumes kept)
yoink doctor                     Diagnose the local setup
yoink help                       Show help
```

Aliases: `start` → `up`, `stop` → `down`. Global flags: `--verbose`, `--quiet`, `--no-color`, `--version`.

A *project* is a locally-managed instance of an initialized repository.
The project name (the repo name, or `--name <name>`) is the identifier every
subsequent command uses; you never repeat the repository URL. When a command
omits the project name, the most recently initialized one is used.

Useful `init` flags: `--name`, `--no-agent` (static-only, no API key),
`--force`, `--output <dir>`, `--no-build`, `--heal-tries N`, `--max-services N`.

URL forms accepted:

- `https://github.com/<owner>/<repo>`
- `https://github.com/<owner>/<repo>.git`
- `https://github.com/<owner>/<repo>/tree/<branch>[/<subdir>]` — clones the
  branch via `--single-branch --branch`
- `git@github.com:<owner>/<repo>[.git]` — normalised to the equivalent
  HTTPS clone URL

## Setup

`yoink setup` is an interactive wizard. It writes
`~/.yoink/config.json` (chmod 0600) with:

```json
{
  "llm_provider": "openai",
  "llm_model":    "gpt-4o",
  "llm_api_key":  "...",
  "github_pat":   "ghp_..."
}
```

Supported providers: OpenAI, Anthropic, Groq, Google Gemini, Ollama
(local — the API key step is skipped and the tool talks to
`localhost:11434`).

The GitHub PAT step is optional. When a PAT is set, `git clone` is invoked
with an `Authorization: Basic` header injected through
`git -c http.extraheader=…`, so the token never appears on the command
line or in `.git/config`. The header config is removed from the cloned
repo after success.

## How detection works

`yoink init` walks the cloned tree, ignoring `node_modules/`, `.venv/`,
`.git/`, `dist/`, `build/`, plus monorepo noise (`examples`, `tests`,
`docs`, …). For each deployable unit it builds a `Service` record:

| field | meaning |
| --- | --- |
| `ID` | Stable id (`service-1`, …) used as compose service name. |
| `Directory` | Path relative to repo root (`""` means root). |
| `Language` | `javascript`, `typescript`, or `python`. |
| `Framework` | `next`, `express`, `fastapi`, … |
| `PackageManager` | `npm` / `yarn` / `pnpm` / `pip` / `poetry`. |
| `InstallCmd`, `BuildCmd`, `StartCmd` | Concrete commands the Dockerfile runs. |
| `Port` | Container port the Dockerfile `EXPOSE`s and compose publishes. |
| `Confidence` | `high` / `medium` / `low`, surfaced in the detection table. |

If an LLM provider is configured and `--no-agent` is not set, the tool sends
the detection and tree to the LLM and accepts corrections to the service
list. The LLM may also request a small number of files (max 5 per round,
16 KiB each). File reads are sandboxed through `internal/safefs` — paths
must be repo-relative and stay inside the cloned root; symlinks pointing
outside are rejected.

## Infrastructure inference

After env-var extraction, Yoink scans the references for patterns that
imply a backing service. Each match adds a compose service with sensible
defaults, a healthcheck, a named volume (for stateful services), and a
`depends_on: service_healthy` edge on each app service that references it.
The connection string is injected into the app's `.env.example`.

| Hint (any of) | Inferred service | Image | Port |
| --- | --- | --- | --- |
| `DATABASE_URL`, `POSTGRES_*`, `PG*` | Postgres | `postgres:16-alpine` | 5432 |
| `MYSQL_*` | MySQL | `mysql:8` | 3306 |
| `REDIS_URL`, `REDIS_*`, `CACHE_URL` | Redis | `redis:7-alpine` | 6379 |
| `MONGO_URI`, `MONGODB_URL`, `MONGO_*` | Mongo | `mongo:7` | 27017 |
| `RABBITMQ_URL`, `AMQP_URL` | RabbitMQ | `rabbitmq:3-management` | 5672, 15672 |
| `ELASTIC_URL`, `ELASTICSEARCH_URL` | Elasticsearch | `elasticsearch:8.13.4` | 9200 |

Existing values in your `.env.example` are preserved — only missing keys
are appended in a clearly labelled `# Backing services inferred by Yoink`
section.

## Build/heal loop

After generation, `yoink init` runs `docker compose build` (unless
`--no-build` is set or Docker isn't installed). If the build fails:

1. The compose output is captured and the failing service is extracted.
2. The Dockerfile + compose + last 80 lines of error are sent to the LLM
   with a "fix this build" prompt.
3. The LLM returns either a corrected Dockerfile, a corrected compose,
   or a "need files" request that triggers another round.
4. The fix is written back to the same `yoink-outputs/Dockerfile.*` /
   `docker-compose.yml` file. Yoink retries the build.
5. Capped at `--heal-tries` attempts (default 3); each iteration is
   surfaced in the completion summary.

You can also run the loop manually against an existing stack:

```sh
yoink heal myrepo            # re-attempt the build/heal loop
yoink heal --tries 5 myrepo  # more attempts
```

## Persistent state

Each initialised repo gets a state directory:

```
~/.yoink/state/<repo>/
├── yoink.lock           # repo metadata, services, port map, detection hash
├── env-overrides.json   # user-edited env values, applied at `yoink up`
└── settings.json        # preferences
```

`yoink.lock` is the authoritative record of what Yoink generated; it lets
`yoink up` and `yoink dash` operate without re-detecting. The
detection-hash field lets re-runs notice when the upstream repo has
changed in a way that invalidates the generated stack.

## Dashboard

```
yoink dash myrepo
```

A multi-pane Bubbletea TUI:

- **Top**: service list with status, health, and bound URL.
- **Middle**: tabbed detail pane.
  - `Logs` — scrollable tail (`pgup`/`pgdown`, `g`/`G` for top/bottom).
  - `Env` — current env file, editable in `$EDITOR` (press `e`).
  - `Controls` — keybinding reminder for the selected service.
- **Bottom**: `docker stats` strip (CPU%, memory, net I/O).

Keybindings:

```
↑/↓      select service
tab      next pane
s        start         x  stop          r  restart      b  rebuild
e        edit env in $EDITOR (writes to env-overrides.json)
g / G    log top / bottom
q        quit
```

Edits made in the dashboard are persisted to `env-overrides.json` and
applied automatically on the next `yoink up`.

## Generated layout

```
yoink-outputs/
├── docker-compose.yml          # one entry per app service + inferred infra
├── Dockerfile.service-1        # one Dockerfile per service
├── Dockerfile.service-2
├── env-vars/
│   ├── service-1/
│   │   ├── .env.example        # template, safe to commit
│   │   └── .env                # real file compose reads, gitignored
│   └── service-2/...
├── quick_start.md
└── .gitignore                  # keeps .env out, lets .env.example through
```

A few things worth knowing:

- Compose build context is `..` (the repo root); each Dockerfile copies
  from its sub-directory (`COPY apps/web/package*.json ./`). Monorepos
  work without ad-hoc tricks.
- `env_file:` references `.env` (not `.env.example`). `yoink up`
  rebuilds `.env` from `.env.example` + per-service overrides before
  starting compose.
- Host port collisions are remapped automatically — the allocator probes
  the host to avoid binding ports that are already in use.
- All Dockerfiles install curl so the compose-level healthcheck works
  out of the box (`curl http://localhost:PORT/`; any HTTP response —
  including 4xx — counts as alive).

## Development

```bash
go build ./...        # build everything
go test ./...         # unit tests (no network, no docker)
go vet ./...          # static analysis
go run . help         # try the binary without installing
make build install    # build with versioning + install
```

The test suite uses temp directories for fixtures, so it's safe to run
anywhere. Tests cover detector framework picks, env-var regex behaviour,
infrastructure inference, healthchecks, port probing, generator output
(YAML parsing, depends_on, volumes), docker output parsing, state
round-trips, the heal loop's fix-application logic, and config /
git / safefs basics.

## License

MIT.
