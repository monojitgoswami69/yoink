# Yoink

Yoink is a CLI tool that takes an existing application repository and turns it into a locally runnable Dockerized application with minimal user intervention. It combines deterministic repository analysis with a bounded AI agent that can inspect the repository, understand build/runtime failures, make constrained patches, iterate, and produce an honest final result.

## What Yoink Does

```
$ yoink init https://github.com/monojitgoswami69/certify
[1/8] Clone repository          cloned (81 files)
[2/8] Generate repository tree   Tree generated
[3/8] Detect services            vite frontend detected
[4/8] Extract environment vars  0 variable references
[5/8] Infer backing services     none
[6/8] Generate Docker config     Dockerfile + compose
[7/8] Write outputs              .env.example + .dockerignore
[8/8] Build & heal              Build succeeded

Init complete in 10.3s
  http://localhost:80/  (HTTP 200)
```

Yoink clones a repository, detects its services, generates Dockerfiles and docker-compose.yml, infers backing infrastructure, repairs build failures through a bounded agent loop, and verifies that the application is actually running and reachable via HTTP.

## Installation

### Option 1: One-line installer (recommended)

```bash
curl -sSfL https://raw.githubusercontent.com/monojitgoswami69/yoink/main/install.sh | sh
```

The installer detects your OS and architecture, downloads the correct pre-built binary from GitHub Releases, and places it in `~/.local/bin` (or `/usr/local/bin` if run as root). It provides PATH instructions tailored to your shell (bash, zsh, fish). If no pre-built binary is available, it falls back to building from source.

### Option 2: Download from GitHub Releases

Go to [Releases](https://github.com/monojitgoswami69/yoink/releases), download the archive for your platform, extract, and move the `yoink` binary to a directory in your PATH.

Archives:
- `yoink_Linux_x86_64.tar.gz`
- `yoink_Linux_arm64.tar.gz`
- `yoink_Darwin_x86_64.tar.gz`
- `yoink_Darwin_arm64.tar.gz`
- `yoink_Windows_x86_64.zip`

### Option 3: Build from source

```bash
git clone https://github.com/monojitgoswami69/yoink.git
cd yoink
go build -o yoink .
mv yoink /usr/local/bin/   # or ~/.local/bin
```

Or use the Makefile:

```bash
make build install
```

Requirements:
- Go 1.25+ to build
- `git` on PATH
- Docker Engine + Compose v2 at runtime (not required for `init --no-build`)

## Setup

```bash
yoink setup
```

This launches an interactive wizard that writes `~/.yoink/config.json` (chmod 0600) with:

```json
{
  "llm_provider": "gemini",
  "llm_model": "gemini-3.1-flash-lite",
  "llm_api_key": "...",
  "github_pat": "ghp_..."
}
```

Supported LLM providers: OpenAI, Anthropic, Groq, Google Gemini, Ollama (local, no API key needed).

The GitHub PAT step is optional. When set, `git clone` uses an `Authorization: Basic` header injected through `git -c http.extraheader`, so the token never appears on the command line or in `.git/config`. The header config is removed after cloning succeeds.

## Commands

```
yoink init <repo>               Clone, detect, generate, build, heal, verify
yoink setup                     Configure LLM provider and GitHub PAT
yoink list [--running|--stopped] List initialized projects
yoink up [project]              Start a project (waits for healthchecks)
yoink down [project]            Stop a project (preserves volumes)
yoink restart [project]         Stop and start with env re-render
yoink status [project]          Show health, services, and URLs
yoink logs <project> [service]  Show or follow container logs
yoink stats <project>           Show CPU, memory, network per service
yoink open [project]            Open the app URL in your browser
yoink env <project>             Manage environment variables
yoink explain [project]         Summarize detection, infrastructure, repairs
yoink heal [project]            Re-run the build/heal loop
yoink update [project]          Pull, regenerate, rebuild, restart
yoink incinerate <project>      Permanently remove a project
yoink doctor                    Diagnose the local environment
yoink help                      Show help
```

Global flags: `--verbose/-v`, `--quiet/-q`, `--no-color`, `--version`.

Hidden aliases: `start` (up), `stop` (down), `remove` (incinerate).

A project is a locally-managed instance of an initialized repository. The project name (the repo name, or a custom `--name`) is the identifier every subsequent command uses. When a command omits the project name, the most recently initialized project is used.

Useful `init` flags: `--name`, `--no-agent` (static-only, no API key), `--force`, `--output <dir>`, `--no-build`, `--heal-tries N`, `--max-services N`.

URL forms accepted by `init`:
- `https://github.com/<owner>/<repo>`
- `https://github.com/<owner>/<repo>.git`
- `https://github.com/<owner>/<repo>/tree/<branch>[/<subdir>]`
- `git@github.com:<owner>/<repo>[.git]`
- Local directory paths (`./`, `/path/to/repo`)

## How Detection Works

`yoink init` walks the cloned repository tree, ignoring `node_modules/`, `.venv/`, `.git/`, `dist/`, `build/`, and monorepo noise directories (`examples`, `tests`, `docs`, etc.). For each deployable unit it builds a Service record with:

| Field | Description |
|---|---|
| ID | Stable identifier (`service-1`, `service-2`, ...) used as compose service name |
| Directory | Path relative to repo root (empty means root) |
| Language | `javascript`, `typescript`, or `python` |
| Framework | `next`, `express`, `fastapi`, `flask`, `vite`, etc. |
| PackageManager | `npm`, `yarn`, `pnpm`, `pip`, `poetry`, `uv` |
| InstallCmd, BuildCmd, StartCmd | Concrete commands the Dockerfile runs |
| Port | Container port the Dockerfile exposes and compose publishes |
| Confidence | `high`, `medium`, or `low` |

Supported frameworks:
- JavaScript/TypeScript: Next.js, Express, Vite, NestJS, Nuxt, Astro, SvelteKit, Remix, CRA, React, generic Node
- Python: FastAPI, Flask, Django, generic Python

If an LLM is configured and `--no-agent` is not set, the detection results and repository tree are sent to the LLM for validation. The LLM can request file reads (max 5 per round, 16 KiB each) through a sandboxed reader that rejects paths outside the repository root.

## Infrastructure Inference

After environment variable extraction, Yoink scans for patterns that imply backing services. Each match adds a compose service with defaults, healthcheck, named volume, and `depends_on: service_healthy` on the referencing app service.

| Environment hint | Inferred service | Image | Port |
|---|---|---|---|
| `DATABASE_URL`, `POSTGRES_*`, `PG*` | Postgres | `postgres:16-alpine` | 5432 |
| `MYSQL_*` | MySQL | `mysql:8` | 3306 |
| `REDIS_URL`, `REDIS_*`, `CACHE_URL` | Redis | `redis:7-alpine` | 6379 |
| `MONGO_URI`, `MONGODB_URL`, `MONGO_*` | Mongo | `mongo:7` | 27017 |
| `RABBITMQ_URL`, `AMQP_URL` | RabbitMQ | `rabbitmq:3-management` | 5672, 15672 |
| `ELASTIC_URL`, `ELASTICSEARCH_URL` | Elasticsearch | `elasticsearch:8.13.4` | 9200 |

External provider detection: when a project uses a provider-specific SDK (e.g. `@neondatabase/serverless`, `@upstash/redis`), the infrastructure is marked as external and Yoink does not provision a local container. Repository-provided provider URLs are preserved and not overwritten with fabricated local hostnames.

## Environment Intelligence

Yoink does not treat every environment variable reference as a requirement. Static discovery produces candidates classified as:

- **PROVIDED_DEFAULT**: repository template supplies a value
- **REQUIRED**: strong evidence the application cannot start without it
- **OPTIONAL**: a default exists or usage is conditional
- **FEATURE_SPECIFIC**: only accessed by a non-core route or integration
- **UNKNOWN**: insufficient evidence to classify

Static references alone never produce REQUIRED. Evidence sources (strongest first):
1. Actual build/runtime failure naming the variable
2. Framework validation indicating required (e.g. pydantic BaseSettings without default)
3. Source code showing unconditional startup access without default
4. Agent semantic investigation after repository file inspection
5. Repository-provided environment templates
6. Static process.env / os.environ references

Repository `.env.example` values are preserved and injected. Obvious placeholders (`placeholder-key-here`, `your-api-key-here`, `change-me`, etc.) are preserved but marked as placeholders, not treated as valid credentials.

## Build and Heal Loop

After generation, `yoink init` runs `docker compose build`. If the build fails:

1. The complete Docker build output is captured and preserved.
2. The failure is classified (compilation, dependency, configuration, missing-environment, nextjs-build, docker-build).
3. Root-cause extraction prioritizes application-level errors (TypeScript, npm, Vite/Rollup, Python, Next.js) over downstream artifacts (`.next not found`, `dist checksum`, BuildKit wrapper errors).
4. Deterministic fixers run first (Python version bump, monorepo sub-package deps, missing env placeholders).
5. If no deterministic fix applies, the AI agent investigates the repository, requests file reads, proposes constrained patches, and Yoink validates and applies them.
6. The agent can only modify generated artifacts (Dockerfiles, docker-compose.yml). Source files, traversal paths, and non-allowlisted filenames are rejected.
7. After patching, Yoink rebuilds and independently verifies runtime health and HTTP reachability.
8. The LLM never declares success. Yoink verifies independently.

Capped at `--heal-tries` attempts (default 3). The agent is bounded by iteration count, tool-call count, bytes read, build count, and duration.

Terminal states and exit codes:
- **SUCCESS** (exit 0): application is running, healthy, and HTTP-reachable
- **CONFIGURATION_REQUIRED** (exit 2): required credentials are unavailable
- **BLOCKED** (exit 3): Yoink cannot resolve the failure within budget
- **FAILED** (exit 4): system error prevents completion

## Docker Build Root-Cause Extraction

Yoink preserves the complete Docker/Compose build output as `RawLog` on the failure struct. The root-cause selector ranks error candidates by specificity:

High priority: TypeScript `TSxxxx`, npm/yarn/pnpm errors, module-not-found, Python ImportError/traceback, Vite/Rollup/esbuild errors, Next.js prerender errors, explicit missing-environment errors, Python version mismatches.

Lower priority: checksum failures, `.next` not found, `dist` not found, BuildKit wrapper errors (`failed to solve`, `returned a non-zero code`).

If no defensible root cause exists, Yoink reports: "root cause could not be determined from Docker build output" rather than guessing.

## Persistent State

Each initialized project gets a state directory:

```
~/.yoink/state/<project>/
├── yoink.lock           # repo metadata, services, port map, detection hash
├── env-overrides.json   # user-edited env values, applied at yoink up
└── settings.json        # preferences
```

`yoink.lock` is the authoritative record of what Yoink generated. It lets `yoink up`, `yoink status`, and `yoink explain` operate without re-detecting.

## Generated Layout

```
<repo>/
├── yoink-outputs/
│   ├── docker-compose.yml
│   ├── Dockerfile.service-1
│   ├── Dockerfile.service-2
│   ├── env-vars/
│   │   ├── service-1/
│   │   │   ├── .env.example        # template, safe to commit
│   │   │   └── .env                # real file compose reads, gitignored
│   │   └── service-2/...
│   ├── quick_start.md
│   └── .gitignore
└── .dockerignore
```

Compose build context is the repo root (`..`). Each Dockerfile copies from its sub-directory. Host port collisions are remapped automatically. All Dockerfiles install curl for healthcheck support.

## Runtime Verification

Build success is not sufficient. Yoink verifies:
- Container state (`running`)
- Docker healthcheck status (`healthy`)
- Host port availability
- HTTP reachability (non-5xx response counts as healthy)
- Application-level rejection (5xx is a failure, 4xx is healthy)

The stack is left running on success. On failure, the stack is torn down and the result is classified as BLOCKED.

## Development

```bash
go build ./...        # build everything
go test ./...         # unit tests (no network, no docker)
go vet ./...          # static analysis
go run . help         # try the binary without installing
make build install    # build with versioning and install
```

Tests cover detector framework picks, env-var regex behavior, infrastructure inference, healthchecks, port probing, generator output (YAML parsing, depends_on, volumes), Docker output parsing, state round-trips, heal loop fix-application logic, root-cause extraction, environment intelligence classification, agent safety (traversal rejection, patch path validation), and config/git/safefs basics.

### Releasing

Releases are automated through GitHub Actions on semantic version tags (`v*`). The workflow runs quality gates (test, vet, build) and then GoReleaser publishes cross-platform binaries (Linux amd64/arm64, macOS amd64/arm64, Windows amd64) to GitHub Releases with checksums.

To create a release:

```bash
git tag v0.1.0
git push origin v0.1.0
```

GoReleaser configuration is in `.goreleaser.yaml`. The release workflow is in `.github/workflows/release.yml`.

## License

MIT.
