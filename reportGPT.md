# Executive Summary

- **Verdict: NOT READY.**
- The deterministic static pipeline is functional and reproducible for the representative repositories.
- `go test ./...`, `go vet ./...`, and `go build` pass. Docker Compose YAML generated for all reachable representatives passes `docker compose config`.
- Docker runtime verification could not be reproduced because the Docker daemon was unavailable in this environment.
- `yoink init` is a real multi-stage pipeline: local/remote materialization, tree generation, detection, LLM validation, environment extraction, infra inference, generation, preflight, optional build/heal, and state persistence.
- The implementation has both a legacy `healer.Loop` and a newer `agent.Agent`; production `init` uses the newer agent only when the initial build fails and an LLM is configured.
- The agent has genuine bounded iteration, deterministic failure parsing, tool declarations, patch application, stale-context hashing, and independent build/runtime verification.
- However, agent tool results are not returned directly to the same model turn. They are merely recorded and partially summarized in later prompts.
- Agent filesystem tools bypass `internal/safefs`; path traversal is possible through `read_file`, `search`, and `list_tree`.
- Agent mutation paths do not robustly constrain filenames. A malicious or malformed model response can write outside `yoink-outputs`.
- Multi-service graph modeling exists but is not integrated into `init` generation. App-to-app relationships are not rewritten into compose environment variables or `depends_on`.
- External-provider handling is incorrect: Neon/Upstash suppress local containers, but generated app env files can still receive local connection strings.
- Existing Dockerfiles and compose files are ignored completely.
- Python detection is dependency-name based and can identify framework presence without proving that the repository actually runs that framework.
- The current `siksha-saathi` run detected the root Next.js app plus `ingestion-worker` as a low-confidence Node service, despite the worker README/package description saying it is deployed separately.
- The historical `rasmalai-hexafalls226` URL was no longer reachable: GitHub returned repository not found, so that case could not be reproduced.

# Architecture

## Actual Packages

| Package | Responsibility | Production path |
|---|---|---|
| `cmd` | Cobra commands and orchestration | Yes |
| `internal/git` | GitHub URL parsing, cloning, local repository resolution | Yes |
| `internal/tree` | Repository tree for LLM context | Yes |
| `internal/detector` | JS/Python/Go/Rust candidate scanning and service records | Yes |
| `internal/envvar` | Source and `.env.example` scanning, classification | Yes |
| `internal/infra` | Dependency/env/provider inference and env injection | Yes |
| `internal/generator` | Dockerfiles, compose, `.dockerignore` content | Yes |
| `internal/preflight` | Generated compose/Dockerfile/env checks | Yes |
| `internal/docker` | Docker/Compose process wrapper | Yes |
| `internal/healer` | Legacy build/heal loop, failure parsing, patch system | Yes, fallback and standalone `yoink heal` |
| `internal/agent` | New bounded agent runtime | Yes, initial `init` build failures with configured LLM |
| `internal/llm` | Provider clients, JSON extraction, validation prompts | Yes |
| `internal/healthcheck` | Compose healthcheck templates | Yes |
| `internal/httpcheck` | Host-level HTTP verification | Yes, final `init` success path |
| `internal/state` | Lock, settings, env overrides, repair provenance | Yes |
| `internal/project` | Shared project resolution, compose handles, health/URL views | Yes |
| `internal/graph` | Evidence-driven service graph used by `explain` | Partially; not integrated into generation |
| `internal/safefs` | Sandboxed reads | Used by validation/healer readers, bypassed by agent tools |
| `internal/config` | Global config | Yes |
| `internal/portprobe` | Host port allocation | Yes |
| `internal/dashboard` | Bubbletea TUI | Yes through `yoink dash` |
| `internal/ui`, `termio` | Presentation and terminal input | Yes |
| `internal/pydeps` | Python native package mapping | Yes |
| `internal/testdata` | Fixture generation | Tests only |

`internal/graph` is a partially integrated abstraction. It models app-to-app URL edges, but `cmd/init.go` never calls `graph.Build`; it only persists infra links and app services in the lock file. The graph is reconstructed by `cmd/explain.go:42-66`.

`internal/healer` and `internal/agent` are duplicated repair architectures. The legacy path remains active when no LLM is available, while `yoink heal` and `yoink update` still instantiate `healer.Loop` directly.

## Text Architecture Diagram

```text
main.go
  -> cmd.Execute()
     -> cmd/root.go
        -> cmd/init.go:runInit()

runInit
  -> git.ParseURL / ParseLocal
  -> config.Load / LoadOptional
  -> git.Clone or local path
  -> tree.Generate
  -> detector.DetectWithCap
  -> optional initAgent.validateDetection
     -> llm.Client.ValidateDetection
     -> safefs.Reader
  -> envvar.Detect
     -> source regex scanning
     -> .env.example scanning
     -> dependency extraction
  -> optional initAgent.validateEnvVars
  -> infra.Infer
     -> env rules
     -> dependency rules
     -> provider rules
  -> generator.Build
     -> Dockerfiles
     -> compose
     -> .dockerignore
  -> optional initAgent.validateDocker
  -> writeOutputs
  -> state.SaveLock
  -> preflight.Check
  -> maybeRunHeal

maybeRunHeal
  -> docker.Compose.Build
  -> if build succeeds:
       Compose.Up
       waitForHealthQuick
       httpcheck.Services
       leave stack running
  -> if build fails and LLM exists:
       agent.Agent.RunHealLoop
         -> deterministic fixer
         -> LLM prompt
         -> tools / patches
         -> Compose.Build
         -> health verification
       -> final Compose.Up
       -> waitForHealthQuick
       -> httpcheck.Services
  -> if no LLM:
       healer.Loop.Run
         -> deterministic fixer
         -> optional legacy LLM repair
         -> build
         -> health verification
  -> final summary
```

# `yoink init` Execution Trace

**FACT:** `cmd/init.go:101-139` parses the repository reference, loads config, and either uses a local path or clones into `~/.yoink/repos/<name>`.

**FACT:** `cmd/init.go:147-164` performs cloning/loading. The clone uses `git.Clone`, shallow clone, optional branch, PAT via `http.extraheader`.

**FACT:** `cmd/init.go:168-183` generates the repository tree and detects services.

**FACT:** `cmd/init.go:205-209` invokes LLM detection validation, but ignores the returned error and continues with static results.

**FACT:** `cmd/init.go:211-239` extracts env vars, optionally asks the LLM to rewrite env output, then seeds common variables.

**FACT:** `cmd/init.go:241-267` performs infrastructure inference and injects inferred connection strings.

**FACT:** `cmd/init.go:270-305` generates artifacts and optionally performs a read-only LLM Docker review. The review is saved to `docker-compose.llm-review.yml` or `Dockerfile.llm-review`; it is deliberately not applied.

**FACT:** `cmd/init.go:307-327` writes generated artifacts and persists the state lock.

**FACT:** `cmd/init.go:329-343` runs preflight. Errors disable the build but do not cause `init` to return an error.

**FACT:** `cmd/init.go:345-356` invokes the build/heal path and always returns success from `runInit` unless an earlier static stage failed.

**RISK:** A preflight error or build/heal failure can result in an exit code of zero because `maybeRunHeal` errors are converted into warnings at `cmd/init.go:347-349`, and `runInit` returns `nil`. The final box may describe failure, but shell automation cannot reliably distinguish success from blocked/failed initialization.

# Detection Quality

## JavaScript / TypeScript

### Evidence and confidence

`internal/detector/js.go:16-80` uses:

- `package.json`
- `dependencies` and `devDependencies`
- scripts
- lockfiles
- `tsconfig.json`
- framework config files
- entrypoint source scanning for ports
- directory-name hints

Framework priority is fixed in `pickJSFramework` at `internal/detector/js.go:113-139`:

```text
next
NestJS
Nuxt
Astro
SvelteKit
Remix
Fastify
Express/Koa/Hapi
Vite
CRA
React
generic Node
```

### Quality ratings

- **Next.js: Medium**
  - Strong exact dependency evidence.
  - False-positive defense requires a `start`/`dev` script or a Next config file.
  - Valid Next applications without `next.config.*` and without a start/dev script are skipped.
  - `next` in `devDependencies` is treated as equivalent to runtime usage.
- **Express: Medium**
  - Exact dependency match and high confidence.
  - A package that includes Express as a dependency and has an unrelated start script can be classified as an Express service.
  - `nodeStart` assumes `npm start` exists or falls back to `main`/`index.js`; it does not validate that the referenced file exists.
- **Vite: Medium**
  - Exact dependency, high confidence.
  - Requires `vite.config.*` if there is no start/dev script, causing false negatives for valid minimal Vite apps.
  - Static generation is sound for standard Vite output, but API URL handling is not inferred from frontend/backend relationships.
- **Generic Node: Low to Medium**
  - Any `start` or `dev` script can cause retention even without framework evidence.
  - The generated command can be `node index.js` even when no `index.js` exists.
- **Package manager: Medium**
  - Lockfile selection is deterministic.
  - npm lockfile handling only recognizes `package-lock.json`; npm projects using another lock mechanism or workspaces can be misclassified.
  - `bun` is detected but the generator has no dedicated Bun base image or runtime semantics.

### Concrete false-positive/negative problems

**FACT:** `detectJS` combines `dependencies` and `devDependencies` at `js.go:41-42`.

**RISK:** A shared package or build-tool package containing `next`, `react`, or `vite` can be classified as an application if it has a start script.

**FACT:** Config-framework filtering at `js.go:49-58` only checks config existence for Next/Nuxt/Vite.

**RISK:** A valid Next/Vite app without a config file and without an explicit start/dev script is dropped.

**FACT:** `skipDirs` in `detector.go:224-260` skips broad names such as `website`, `scripts`, `tools`, and `fixtures`.

**RISK:** A legitimate nested deployable service named `website` or `tools` is silently excluded.

**FACT:** Candidate scanning does not inspect existing Dockerfiles or compose files.

**RISK:** A repository whose deployable entrypoint is defined only by Docker configuration may be missed.

## Python

### Quality ratings

- **FastAPI: Medium**
- **Flask: Medium**
- **Django: Low to Medium**
- **Generic Python: Low**

**FACT:** `internal/detector/python.go:25-38` classifies based on substring presence in a comment-stripped manifest.

**RISK:** Exact package parsing is not used. Names such as `fastapi-users`, an unrelated package containing `flask`, or arbitrary TOML keys can trigger framework detection.

**FACT:** Comments are stripped, which prevents simple `# uses fastapi` false positives.

**FACT:** `findFastAPIEntry` and `findFlaskEntry` scan source for regex matches such as `app = FastAPI(` and `app = Flask(`.

**RISK:** They can select an application instance from an example or auxiliary file because there is no explicit application-root/source-priority model beyond filename/depth scoring.

**FACT:** Django always produces `python manage.py runserver 0.0.0.0:8000` at `python.go:76-84`.

**RISK:** A Django repository without `manage.py` receives a start command that cannot work.

**FACT:** Python detection never rejects a candidate, including a directory containing only a manifest.

**RISK:** Documentation, packaging, or library directories with `pyproject.toml` become deployable services.

## Monorepos

The detector handles nested manifests and skips common noise directories. It does not model workspaces as a first-class graph.

The `siksha-saathi` run demonstrates the limitation:

```text
service-1: ingestion-worker / node / low
service-2: repository root / next / high
```

The worker package explicitly says it is deployed separately on Render, but detector semantics treat every nested package with a start/dev script as a service.

# Environment Intelligence

## What Is Scanned

**JavaScript patterns:** `internal/envvar/envvar.go:17-27`

- `process.env.X`
- `process.env["X"]`
- `import.meta.env.X`
- `import.meta.env["X"]`
- `NEXT_PUBLIC_*`
- `VITE_*`
- `REACT_APP_*`

**Python patterns:** `envvar.go:29-41`

- `os.environ.get`
- `os.environ[...]`
- `os.getenv`
- bare `getenv`
- `env`
- `config`

**Additional sources:**

- `.env.example`
- `.env.sample`
- `.env.template`
- `.env.local.example`
- Pydantic `BaseSettings` fields

The scanner skips generated and dependency directories at `envvar.go:43-60`.

## Classification

**FACT:** Classification is name-based at `envvar.go:120-151`.

- Public: `NEXT_PUBLIC_*`, `VITE_*`, `REACT_APP_*`
- Secret: names containing `SECRET`, `PASSWORD`, `TOKEN`, `CREDENTIAL`, `PRIVATE_KEY`, `API_KEY`
- Everything else: private

**RISK:** Names such as `DATABASE_URL`, `GEMINI_MODEL`, or `R2_ACCOUNT_ID` are classified as private even when they are credentials or externally sensitive.

**RISK:** Names such as `PUBLIC_API_KEY` are classified as public due to the `API_KEY` marker only if the public-prefix branch does not match first; classification order matters and is heuristic, not semantic.

## Build-Time Classification

**FACT:** `assessBuildTime` at `envvar.go:162-181` only marks JavaScript variables as build-time when the same file contains one of:

- `getStaticProps`
- `getStaticPaths`
- `generateStaticParams`
- `generateMetadata`
- `getServerSideProps`

**RISK:** Next.js top-level module evaluation, route handlers, server components, middleware, imported configuration, and arbitrary build-time code can access env vars without any listed function. These are missed.

**FACT:** Python variables are never marked build-time.

**FACT:** `cmd/init.go:273-285` injects every parsed env value into Dockerfile `ENV` directives, not only variables classified as build-time.

**RISK:** This increases image-layer exposure and makes classification mostly informational. Secret values are replaced by placeholders by `generator.writeBuildEnv`, but non-secret sensitive values can still be embedded.

## No `.env` in GitHub Repository

Static analysis can reliably discover explicit references:

- direct env access
- framework public prefixes
- committed env templates
- Pydantic field declarations
- some dependency-based infrastructure clues

It cannot reliably discover:

- dynamically constructed variable names
- values loaded through arbitrary config libraries
- runtime-only requirements not reached by static regexes
- secrets whose names are opaque
- build-time requirements triggered only by execution
- external provider credentials unless dependency/env naming reveals the provider

The agent is needed when the build/runtime failure exposes a missing requirement. However, the current agent cannot safely infer a secret value, correctly classify all opaque variables, or always inspect the relevant source because of the tool/context limitations described below.

# Infrastructure and Providers

## Neon / Upstash

**FACT:** Provider dependencies are recognized in `infra.go:170-187`.

```text
@neondatabase/serverless -> postgres / external / neon
@vercel/postgres         -> postgres / external / vercel
@upstash/redis           -> redis / external / upstash
```

**FACT:** `infra.Infer` marks provider-backed infrastructure as external at `infra.go:93-109`.

**FACT:** Compose suppresses external containers at `generator/compose.go:73-80`.

**BUG:** `buildAppEnv` still generates local connection values for external infra. `infra.go:107` calls it for the external service, and `cmd/init.go:253-267` enriches `.env.example` with those values.

The generated `siksha-saathi` result demonstrates this:

```text
Inferred: postgres (@neondatabase/serverless indicates neon)
Generated app env:
DATABASE_URL=postgresql://app:app@postgres:5432/app
```

There is no local `postgres` container in the compose output, so the generated value points to a nonexistent hostname.

The same problem appears in the `portfolio` run for Upstash: local Redis values are injected even though Redis is not provisioned.

**Recommendation:** External provider mode must preserve the repository’s original connection-string variable/value or leave it blank and classify it as configuration-required. It must never inject a local hostname for an external-only service.

## Dependency Inference

**FACT:** `infra.go:521-531` maps dependencies such as `pg`, `sqlalchemy`, `redis`, `mongoose`, and `@elastic/elasticsearch` to local infra.

**RISK:** Dependency presence alone is not proof of runtime usage. A package installed for migrations, tests, optional features, or a shared library can cause redundant containers.

**RISK:** A generic `pg` dependency plus Neon provider detection suppresses local Postgres correctly, but env injection still becomes wrong.

**RISK:** Provider imports performed through a wrapper package or transitive dependency are not detected.

# Docker Generation

## Supported MVP Output

The generators exist for:

- Next.js
- Express/Fastify/generic Node
- Vite/React/CRA/Astro
- FastAPI
- Flask
- Django
- additional Go/Rust paths outside the requested MVP

**FACT:** `generator.Build` rewrites React/Vite/CRA/Astro runtime port to 80 at `dockerfile.go:34-48`, using nginx.

**FACT:** Compose publishes per-service host ports through `portprobe.New()`.

**FACT:** Generated compose uses a repo-root build context and output-relative Dockerfile paths at `compose.go:96-143`.

**FACT:** Generated healthchecks use `curl` and accept all HTTP statuses including 5xx at `healthcheck.go:43-57`.

**FACT:** Final `httpcheck` rejects 5xx but accepts 2xx, 3xx, 4xx at `httpcheck.go:45-66`.

## Material Generation Risks

- `renderNode` runs the detected build command but does not verify that the command’s output matches the start command.
- `nodeStart` defaults to `node index.js`; no existence check occurs.
- `renderReact` assumes Vite output is `dist` and CRA output is `build`; this is correct for defaults but does not inspect custom output configuration.
- Next.js runtime uses `.next`, `node_modules`, `package.json`, staged config, and public files. The standalone output optimization is not used, so images may be materially larger.
- Python `pyproject.toml` handling installs the entire project with `pip install .`, which fails for application-only projects lacking a valid build backend.
- Poetry installs dependencies before copying source, but the generator does not verify that `poetry.lock` and `pyproject.toml` are compatible.
- `curl` is installed in every generated runtime image, increasing image size but helping healthcheck reliability.
- Infra services publish host ports unnecessarily. This increases collision risk and exposes databases/caches on localhost.
- `preflight.Check` only detects duplicate ports within generated YAML. It does not validate Dockerfile syntax, image availability, build context existence, healthcheck executable availability, or provider/env consistency.

## Representative Static Runs

| Repository | Current detection | Infra | Static artifact result |
|---|---|---|---|
| `certify` | 1 Vite frontend | none | Generated valid compose |
| `nohara-family-webmaniacs` | 1 Next frontend | none | Generated valid compose |
| `portfolio-website` | 1 Next frontend | Upstash Redis external | Local Redis URL incorrectly injected |
| `Sevatra` | 3 Vite frontends + 1 FastAPI backend | local Postgres + Redis | Generated 4-app compose with healthchecks |
| `siksha-saathi` | root Next + low-confidence Node worker | Neon Postgres external | Worker likely incorrectly included; local Postgres URL incorrectly injected |
| `rasmalai-hexafalls226` | not verified | not verified | GitHub repository URL returned 404 |

All generated compose files passed `docker compose config --quiet`. No image build or runtime start was possible because the daemon was unavailable.

# Existing Docker Intelligence

There is effectively none.

**FACT:** Repository scanning in `detector/detector.go:124-151` only recognizes:

- `package.json`
- Python manifests
- `go.mod`
- `Cargo.toml`

**FACT:** No production code searches for or parses:

- `Dockerfile`
- `docker-compose.yml`
- `docker-compose.yaml`
- `compose.yaml`
- `docker-compose.override.yml`

**FACT:** `generator.Build` always emits new Dockerfiles and compose.

**FACT:** `preflight.Check` validates only Yoink-generated artifacts.

**Verdict:** Ignoring existing Docker configuration is MVP-critical for repositories that already contain authoritative build context, services, networks, volumes, commands, or environment wiring. It is not necessarily a requirement to support arbitrary compose augmentation tonight, but Yoink must at minimum detect existing Docker configuration and either reuse it explicitly or report that it is intentionally replacing it.

# Agent Architecture

## Real Agent or Decorative Loop?

It is a real bounded loop, but incomplete.

**FACT:** `agent.Agent.RunHealLoop` at `agent.go:183-287`:

1. Parses the initial failure.
2. Tracks iteration/build/time budgets.
3. Applies deterministic fixes first.
4. Calls an LLM.
5. Parses structured changes or full replacements.
6. Rebuilds.
7. Independently verifies runtime health.
8. Produces a final state.

**FACT:** The legacy `healer.Loop.Run` at `healer.go:88-372` separately implements a build, context pack, LLM repair, patching, stale detection, and final verification loop.

## Context Problems

**BUG:** `agentIteration` executes tool calls and discards their returned results at `agent.go:538-546`:

```go
result := a.executeToolCall(ctx, tc)
_ = result // fed back in the next iteration
```

The next prompt only includes the last three tool results in truncated form at `agent.go:1043-1052`. The model does not receive each tool result as a direct assistant/tool exchange. This makes tool use slower and less reliable than a normal tool-calling loop.

**BUG:** The agent context often describes only the first service:

- `buildContextPrompt` uses `a.State.Services[0]` at `agent.go:978-987`.
- `agentIteration` passes the first service into `BuildContextPack` at `agent.go:1018-1026`.

For a multi-service failure, the failure service may be `service-3`, while the prompt contains metadata and relevant files for `service-1`.

**RISK:** The agent may patch the wrong Dockerfile or reason from the wrong package manifest.

**FACT:** The newer agent tools are:

- `read_file`
- `search`
- `list_tree`
- `build`
- `get_logs`
- `check_health`

They are represented in the prompt, but only the read/search/list tools are useful for initial repository investigation. The model cannot inspect arbitrary command output, run package managers, inspect a process environment, or query compose configuration beyond the provided wrappers.

## State and Progression

**FACT:** Failure fingerprints and progression classification exist at `healer/failure.go:59-93`.

**FACT:** Previous patches and tool history are included in later context.

**RISK:** The agent tracks patch records but not a complete structured diff/result for every attempted patch. `PatchRecord` stores file, operation, reason, and timestamp, not the before/after hashes used by the legacy loop.

**RISK:** Initial `initAgent` validation calls and the actual heal agent are separate LLM conversations. Detection/env/Docker validation conclusions are not carried into `AgentState` as a durable reasoning record.

# Patch and Safety System

## What Is Constrained

`healer.Change` and `ApplyPatch` support:

- `insert_after`
- `insert_before`
- `replace_line`
- `replace_exact`
- `create_file`

Anchors are required and must be unique for non-create operations at `healer/repair.go:84-185`.

`CheckInvariants` rejects:

- disabling TypeScript checks
- removing healthchecks
- replacing commands with sleep
- removing dependency installation

`EvaluateScope` rejects changes over 50 lines and forbidden sleep/CMD changes.

## Critical Safety Gaps

**BUG:** `agent.toolReadFile`, `toolSearch`, and `toolListTree` construct paths using `filepath.Join(a.State.RepoRoot, path)` instead of `safefs.Reader.Read` at `agent.go:641-745`.

The `../` traversal and symlink containment guarantees in `internal/safefs` therefore do not apply to agent tools.

**BUG:** Agent patch paths are not fully constrained to generated files. `applyAgentPatches` normalizes only the `yoink-outputs/` prefix at `agent.go:789-803`, then writes:

```go
filepath.Join(a.State.OutputDir, file)
```

A model-supplied filename containing traversal segments can target outside the output directory.

**BUG:** The full-file fallback permits a model-selected service name to participate in:

```go
filepath.Join(a.State.OutputDir, dfKey)
```

at `agent.go:565-575` and `884-895`.

**BUG:** The fallback path unconditionally processes a Dockerfile key even when the response only contains a compose replacement. `agent.go:557-576` computes `cleaned := llm.CleanContent(fix.Dockerfile)` and writes a Dockerfile even when `resp.Dockerfile` is empty.

**FACT:** Source-file modifications are only a warning in `CheckInvariants` at `repair.go:261-268`, and `create_file` changes bypass that warning.

**Verdict:** The safety system is meaningful for normal well-formed LLM responses, but it is not robust enough to claim that all mutation is constrained.

# Multi-Service Architecture

## Current Model

`infra.AppLink` models app-to-infrastructure relationships.

`graph.ServiceGraph` models:

- app nodes
- local infra nodes
- external infra nodes
- app-to-infra dependency edges
- limited app-to-app URL edges

The app-to-app edge logic at `graph.go:258-299` only detects environment values containing an explicit URL with a numeric port. It maps by port and refuses ambiguous matches.

## What Is Missing

**FACT:** `generator/compose.go:96-143` adds `depends_on` only for infra links.

**FACT:** No code rewrites frontend env values such as:

```text
VITE_API_URL=http://localhost:8000
```

to:

```text
VITE_API_URL=http://service-3:8000
```

inside the container.

**FACT:** Browser-facing variables often need `localhost:<host-port>`, while server-side variables need Docker DNS `<service-name>:<container-port>`. The current model does not distinguish those contexts during generation.

**FACT:** `graph.Build` is not called during `init`, so even its limited app-to-app inference is not used to generate compose.

**Conclusion:** The current compose network is sufficient for containers to resolve each other by service name, but Yoink does not reliably configure applications to use that network. Frontend A -> backend B and backend B -> database are only partially supported. Frontend-to-backend browser URLs may accidentally work with host-published ports, but server-to-server links generally do not.

This is the likely current bottleneck for the historical multi-service blocked cases, not merely LLM quality.

# Runtime Verification

Implemented checks:

- container state: `State == "running"`
- Docker health status: `healthy`, `unhealthy`, `starting`
- host port availability indirectly through HTTP probes
- TCP/listener reachability through HTTP client behavior
- HTTP status code
- application-level 5xx rejection

`internal/httpcheck/httpcheck.go:45-66` rejects connection failures, timeouts, and 5xx responses.

Limitations:

- only `/` is probed
- no framework-specific route selection
- 404 is treated as healthy
- no response-body validation
- no browser-level verification
- infra container HTTP is not checked by `httpcheck`
- a service with no published port can pass if Docker healthchecks pass
- `waitForHealthQuick` requires every container to report health; a compose service without a healthcheck remains “starting” until timeout

The HTTP verification addition is worthwhile and materially improves the MVP. The current issue is that generated healthchecks accept 5xx while final host probes reject them, creating two different health definitions.

# CLI / UX

Implemented commands match the documented surface:

- `init`
- `up`
- `down`
- `status`
- `logs`
- `stats`
- `dash`
- `env`
- `doctor`
- `list`
- `open`
- `heal`
- `update`
- `config`
- `remove`
- `restart`
- `explain`
- `setup`
- `help`

`yoink help`, `yoink --version`, and `yoink doctor` ran successfully. The output is coherent, progress-oriented, and exposes URLs in the normal success paths.

Important UX issues:

- `init` can exit zero after a failed build/heal because errors are downgraded to warnings.
- `CONFIGURATION_REQUIRED`, `BLOCKED`, and `FAILED` are not represented as process exit statuses.
- The final summary is not always explicit when Docker is unavailable; it can simply say “Skipping build/heal.”
- `open` opens the configured URL even when the stack is stopped.
- `status` reports “unknown” and exits zero when Docker is unavailable.
- `env set KEY=value` applies the variable to every service declaring it, or the first service otherwise. There is no explicit service selector.
- `yoink update` returns `nil` after a failed build when healing does not succeed at `update.go:164-168`.
- `dashboard.Run` uses `yoink-` plus `lock.Repo` rather than canonical project naming at `dashboard.go:28-30`, while `project.fromManager` uses canonicalized project naming. This can cause dashboard compose-project mismatches for mixed-case/custom names.

# Test Quality

The test suite is strong for unit-level deterministic behavior but weak for the product contract.

Covered:

- detector adversarial cases
- env regex behavior
- infra/provider inference
- compose YAML shape
- Dockerfile substrings
- healthcheck templates
- HTTP checks
- state round trips
- patch application
- stale snapshots
- deterministic fixes
- config/git/safefs basics

Not covered adequately:

- actual Docker builds
- actual Docker runtime
- real LLM conversations
- model tool-call result propagation
- agent path traversal
- agent multi-service context selection
- external provider env correctness
- existing Dockerfile/compose behavior
- Next.js build-time env failures
- `init` exit codes for blocked/configuration-required/failed states
- generated Dockerfiles against the historical repositories
- browser-side frontend-to-backend connectivity
- compose with custom output directories
- Docker Compose v1 fallback behavior
- worker/package exclusion semantics

The suite can produce false confidence because most generator tests check substrings and YAML parsing rather than building the produced artifacts.

# Performance / Demo Practicality

Static initialization was effectively instantaneous on the representative repositories.

Expected expensive operations:

- repository clone
- Docker image pulls
- Node dependency installation
- Python dependency installation
- Next.js builds
- up to three initial validation LLM calls
- up to three agent repair iterations
- up to four agent builds
- 60-second health waits
- 5-second HTTP probes
- LLM request timeout of 180 seconds at `llm/client.go:14-18`

A worst-case demo can therefore take:

```text
3 validation calls
+ initial Docker build
+ 3 agent calls
+ up to 4 rebuilds
+ repeated 60-second runtime waits
```

This can easily exceed several minutes. The tool does not have a clear demo mode that skips validation calls or caps wall-clock time more aggressively.

# What Already Works

- Deterministic service records carry framework, language, PM, commands, ports, confidence, and evidence.
- JS/TS nested service detection works for the public representatives tested.
- Vite, Next.js, FastAPI, and mixed Vite/FastAPI static generation works structurally.
- Host-port allocation and compose YAML generation are deterministic.
- `.dockerignore`, `.env.example`, and per-service output layout are generated.
- Secrets are not emitted verbatim into Dockerfile `ENV` directives.
- Provider dependencies such as Neon and Upstash are recognized.
- Docker healthchecks and independent HTTP verification both exist.
- State persistence supports later `up`, `status`, `open`, `dash`, and `explain`.
- Repair provenance is persisted and considered during `update`.
- The agent has bounded budgets and independent verification.
- `go test`, `go vet`, and `go build` pass.

# What Is Genuinely Impressive

- The codebase has moved beyond a simple Dockerfile generator into deterministic detection, structured failure analysis, patch validation, and runtime verification.
- The legacy healer has stale-context protection across generated and relevant source files.
- The system explicitly refuses to fabricate secrets.
- The generated compose network and host-port allocation are practical for basic multi-service stacks.
- `explain` reconstructs a useful evidence-oriented view from persisted state without re-running the LLM.

# What Can Fail During a Demo

- Neon or Upstash projects receive local connection strings despite no local provider container.
- A low-confidence nested worker or tooling package is treated as a runnable service.
- Existing Docker configuration is ignored and replaced.
- Frontend/backend services are generated without correct internal/external URL wiring.
- The agent investigates one service while the failure belongs to another.
- Tool calls appear to run but their results are not directly available to the model until a later iteration.
- Agent path traversal or malformed filenames can escape the intended mutation scope.
- Agent compose-only full replacements can create an empty Dockerfile.
- Generated Python start commands can target nonexistent modules or `manage.py`.
- Build failures can still produce exit code zero from `init` or `update`.
- Docker healthchecks can pass on 5xx while final HTTP verification rejects the application.
- Docker daemon absence silently skips the core differentiator unless the user explicitly runs `doctor`.
- Runtime verification is unavailable in environments where Docker is installed but not running.
- Huge dependency builds can make the demo appear stalled.

# MVP Fitness Verdict

## NOT READY

The project is a credible static generator with a meaningful agent/healer architecture, but the current implementation is not reliable enough for an autonomous “unfamiliar repository to runnable application” demo tomorrow. The provider bug, multi-service wiring gap, agent context bug, mutation-scope bypasses, and zero-exit failure behavior are all directly visible in the primary user journey.

# Critical Fixes Before Demo

## P0

1. **Fix external-provider env injection**
   - Problem: `infra.Infer` marks Neon/Upstash external but `cmd/init.go:253-267` injects local hostnames.
   - Area: `internal/infra/infra.go`, `cmd/init.go`, `generator`.
   - Result: External provider projects preserve original connection variables and become `CONFIGURATION_REQUIRED` when values are absent.
   - Verify: Run `portfolio` and `siksha-saathi`; no `redis`/`postgres` local hostname may be injected for external-only providers.

2. **Close agent filesystem and mutation traversal**
   - Problem: `internal/agent/agent.go:641-745` bypasses `safefs`; patch paths are not contained.
   - Area: `internal/agent/agent.go`, `internal/safefs`.
   - Result: All reads remain repository-relative; all writes remain generated-output-relative and allowlisted.
   - Verify: Unit tests for `../`, absolute paths, symlink paths, `yoink-outputs/../`, and service-name traversal.

3. **Fix multi-service agent context**
   - Problem: `agent.go:978-1026` uses the first service instead of the failing service.
   - Area: `internal/agent/agent.go`.
   - Result: The agent receives the correct manifest, Dockerfile, framework, and source files.
   - Verify: Mock a `service-3` failure and assert the prompt contains service-3 metadata/files.

4. **Return tool results to the same agent conversation**
   - Problem: `agent.go:538-546` discards tool results.
   - Area: `internal/agent/agent.go`.
   - Result: Tool calls become actual iterative observe/reason steps.
   - Verify: Mock LLM response requesting `read_file`, then assert the next model prompt contains the complete read result.

5. **Prevent empty Dockerfile writes on compose-only responses**
   - Problem: `agent.go:557-576` writes a Dockerfile even when only compose is returned.
   - Area: `internal/agent/agent.go`.
   - Result: Each returned artifact is handled independently.
   - Verify: Compose-only model response preserves the existing Dockerfile byte-for-byte.

6. **Make terminal states affect exit codes**
   - Problem: `runInit` and `update` return zero after blocked/failed builds.
   - Area: `cmd/init.go`, `cmd/update.go`, possibly `agent.Result`.
   - Result: `SUCCESS` exits 0; `CONFIGURATION_REQUIRED`, `BLOCKED`, and `FAILED` have distinct nonzero behavior or documented stable codes.
   - Verify: Shell tests for each terminal state.

## P1

1. Detect existing Docker configuration and explicitly classify it as reused, ignored, or replaced.
2. Add a narrow service-selection policy for nested packages, including an explicit “separately deployed” signal and low-confidence confirmation.
3. Integrate `graph.Build` into generation for app-to-app edges where evidence is unambiguous.
4. Separate browser-facing host URLs from server-side Docker DNS URLs.
5. Add real Docker integration tests for at least one Next.js, Vite, Express, and FastAPI fixture.
6. Add external-provider integration tests for Neon and Upstash.
7. Make healthcheck policy consistent with HTTP verification, or document that compose health means listener-ready while final verification means application-ready.
8. Fix `update` to return failure when build/heal does not succeed.
9. Ensure dashboard compose project naming uses the same canonical project name as all other commands.
10. Add a bounded demo mode or reduce unnecessary pre-build LLM validation calls.

## P2

1. Improve Python manifest parsing using structured TOML/requirements parsing.
2. Improve build-time env analysis beyond function-name heuristics.
3. Add framework-specific HTTP probe route configuration.
4. Improve image size with Next.js standalone output where safely supported.
5. Add response-body or route-level checks for known application types.
6. Add full repository acceptance tests that run detection and generation against pinned fixtures.

# Recommended Execution Plan

## PHASE 1: Make the Primary Path Honest and Safe

**Problem:** The current command can generate incorrect external-provider env values, let agent paths escape scope, and report failed initialization with a successful process exit.

**Implementation areas:**

- `internal/infra/infra.go`
- `cmd/init.go`
- `internal/agent/agent.go`
- `internal/safefs`
- `cmd/update.go`

**Expected result:**

- External provider projects do not receive local connection strings.
- Agent reads are sandboxed.
- Agent writes are allowlisted and contained.
- Terminal state is reflected in exit behavior.

**Verification:**

- Unit tests for traversal and provider cases.
- Disposable `portfolio` and `siksha-saathi` runs.
- Shell-level exit-code tests.

## PHASE 2: Make the Agent Actually Useful

**Problem:** The agent currently has a tool protocol, but tool results are not returned directly and multi-service prompts often describe the wrong service.

**Implementation areas:**

- `internal/agent/agent.go`
- `internal/healer/context.go`
- `internal/healer/failure.go`

**Expected result:**

```text
failure identifies service-3
-> prompt contains service-3 metadata
-> model requests source file
-> tool result is returned immediately
-> model proposes patch
-> Yoink validates/applies patch
-> build and runtime are independently verified
```

**Verification:**

- Mock LLM conversation tests.
- Multi-service failure fixture.
- Assert tool history, prompt content, patch provenance, and rebuild count.

## PHASE 3: Stabilize the Demo Repository Set

**Problem:** The static generator works on the easy representatives, but multi-service and provider cases still have material correctness gaps.

**Implementation areas:**

- `internal/detector`
- `internal/graph`
- `internal/generator`
- `internal/preflight`
- acceptance fixtures

**Expected result:**

- `certify`: Vite success.
- `nohara`: Next success.
- `portfolio`: honest configuration-required result with no local Redis.
- `Sevatra`: four-service generation with correct backend dependencies.
- `siksha-saathi`: root Next app only unless worker is explicitly selected; Neon remains external.
- Existing Docker repos are explicitly classified.

**Verification:**

- Run each repository through `--no-agent --no-build`.
- With Docker available, build and start at least `certify`, `nohara`, and `Sevatra`.
- Use a mock LLM for deterministic healing tests.
- Record detected services, generated artifacts, build attempts, final state, and runtime URLs.

# Final Answer

I would **not confidently demo the current implementation to hackathon judges tomorrow**.

The exact changes I would make tonight are:

1. Fix external-provider env injection.
2. Close agent read/write path traversal.
3. Correct the agent’s selected-service context.
4. Return tool results directly to the model.
5. Fix compose-only agent responses.
6. Make blocked/configuration-required/failed states produce honest exit behavior.
7. Add one end-to-end Docker test for a Vite app and one for the Sevatra-style multi-service case.
8. Explicitly detect and report existing Docker configuration rather than silently replacing it.

I would defer broader language support, Kubernetes, dashboards beyond the existing TUI, and general architectural cleanup until after these fixes.
