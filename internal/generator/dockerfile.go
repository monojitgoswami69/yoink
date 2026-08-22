// Package generator builds Dockerfiles and docker-compose.yml from detected services.
package generator

import (
	"fmt"
	"sort"
	"strings"

	"yoink/internal/detector"
	"yoink/internal/healthcheck"
	"yoink/internal/pydeps"
)

// Output bundles everything the writer needs to lay down on disk.
type Output struct {
	// Files maps relative paths (within yoink-outputs/) to file contents.
	Files map[string]string
	// Dockerignore is the content of a .dockerignore file that must live at
	// the repository root (the compose build context is ".."). It keeps the
	// context lean and prevents host node_modules/.venv/.git from being
	// copied into images via `COPY . ./`.
	Dockerignore string
}

// Build constructs the full set of generated files. The output sub-directory
// is set by opts.OutputSubdir; the compose file lives at
// <outputSubdir>/docker-compose.yml with all paths relative to it.
//
// Static-served SPAs are rewritten to bind port 80 (nginx). Inferred infra
// services in opts.Infra are appended to the compose file.
func Build(services []detector.Service, opts Options) *Output {
	out := &Output{Files: map[string]string{}, Dockerignore: DockerignoreContent()}

	// Copy services so we can adjust runtime ports without mutating the caller's slice.
	runtime := make([]detector.Service, len(services))
	copy(runtime, services)
	for i := range runtime {
		switch runtime[i].Framework {
		case "react", "vite", "cra", "astro":
			runtime[i].Port = 80
		}
	}

	for _, svc := range runtime {
		out.Files["Dockerfile."+svc.ID] = renderDockerfile(svc)
	}
	out.Files["docker-compose.yml"] = renderCompose(runtime, opts)
	return out
}

// renderDockerfile generates a Dockerfile assuming the build context is the
// repository root. Paths are sub-directory aware so monorepos work cleanly.
func renderDockerfile(svc detector.Service) string {
	prefix := ""
	if svc.Directory != "" {
		prefix = strings.TrimSuffix(svc.Directory, "/") + "/"
	}

	switch svc.Framework {
	case "next":
		return renderNext(svc, prefix)
	case "nest":
		return renderNest(svc, prefix)
	case "react", "vite", "cra", "astro":
		return renderReact(svc, prefix)
	case "express", "fastify", "node", "nuxt", "sveltekit", "remix":
		return renderNode(svc, prefix)
	case "fastapi":
		return renderFastAPI(svc, prefix)
	case "flask":
		return renderFlask(svc, prefix)
	case "django":
		return renderDjango(svc, prefix)
	case "go":
		return renderGo(svc, prefix)
	case "rust":
		return renderRust(svc, prefix)
	default:
		if svc.Language == "python" {
			return renderPython(svc, prefix)
		}
		return renderNode(svc, prefix)
	}
}

func renderNext(svc detector.Service, prefix string) string {
	pm := svc.PackageManager
	if pm == "" {
		pm = "npm"
	}
	build := joinArgs(svc.BuildCmd)
	if build == "" {
		build = pm + " run build"
	}
	install := safeInstallCmd(svc)

	var b strings.Builder
	header(&b)
	fmt.Fprintln(&b, "FROM node:20-alpine AS deps")
	fmt.Fprintln(&b, "WORKDIR /app")
	fmt.Fprintf(&b, "COPY %spackage*.json ./\n", prefix)
	if lock := copyLockfile(prefix, pm, svc.HasLockfile); lock != "" {
		fmt.Fprintln(&b, lock)
	}
	fmt.Fprintf(&b, "RUN %s\n", install)

	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "FROM node:20-alpine AS builder")
	fmt.Fprintln(&b, "WORKDIR /app")
	fmt.Fprintln(&b, "COPY --from=deps /app/node_modules ./node_modules")
	fmt.Fprintf(&b, "COPY %s. ./\n", prefix)
	writeBuildEnv(&b, svc)
	fmt.Fprintf(&b, "RUN %s\n", build)
	// Stage next.config.* and public/ (if any) into /staged so the runner
	// can COPY a guaranteed-non-empty dir. BuildKit rejects COPY globs or
	// directory sources that match nothing, so we never copy bare
	// /app/next.config.* or /app/public in the runner.
	fmt.Fprintln(&b, "RUN mkdir -p /staged/cfg /staged/pub && (cp /app/next.config.* /staged/cfg/ 2>/dev/null || true) && (cp -r /app/public/* /staged/pub/ 2>/dev/null || true) && touch /staged/cfg/.keep /staged/pub/.keep")

	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "FROM node:20-alpine AS runner")
	fmt.Fprintln(&b, "ENV NODE_ENV=production")
	fmt.Fprintln(&b, "WORKDIR /app")
	curlInstall(&b, svc.Framework)
	fmt.Fprintln(&b, "COPY --from=builder /app/.next ./.next")
	fmt.Fprintln(&b, "COPY --from=builder /app/node_modules ./node_modules")
	fmt.Fprintln(&b, "COPY --from=builder /app/package.json ./")
	fmt.Fprintln(&b, "COPY --from=builder /staged/pub/ ./public/")
	fmt.Fprintln(&b, "COPY --from=builder /staged/cfg/ ./")
	fmt.Fprintf(&b, "EXPOSE %d\n", svc.Port)
	fmt.Fprintln(&b, dockerCMD(svc.StartCmd))
	return b.String()
}

func renderNest(svc detector.Service, prefix string) string {
	pm := svc.PackageManager
	if pm == "" {
		pm = "npm"
	}
	install := safeInstallCmd(svc)

	var b strings.Builder
	header(&b)
	fmt.Fprintln(&b, "FROM node:20-alpine AS deps")
	fmt.Fprintln(&b, "WORKDIR /app")
	fmt.Fprintf(&b, "COPY %spackage*.json ./\n", prefix)
	if lock := copyLockfile(prefix, pm, svc.HasLockfile); lock != "" {
		fmt.Fprintln(&b, lock)
	}
	fmt.Fprintf(&b, "RUN %s\n", install)

	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "FROM node:20-alpine AS builder")
	fmt.Fprintln(&b, "WORKDIR /app")
	fmt.Fprintln(&b, "COPY --from=deps /app/node_modules ./node_modules")
	fmt.Fprintf(&b, "COPY %s. ./\n", prefix)
	writeBuildEnv(&b, svc)
	fmt.Fprintf(&b, "RUN %s\n", joinArgs(svc.BuildCmd))

	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "FROM node:20-alpine AS runner")
	fmt.Fprintln(&b, "ENV NODE_ENV=production")
	fmt.Fprintln(&b, "WORKDIR /app")
	curlInstall(&b, svc.Framework)
	fmt.Fprintln(&b, "COPY --from=builder /app/dist ./dist")
	fmt.Fprintln(&b, "COPY --from=builder /app/node_modules ./node_modules")
	fmt.Fprintln(&b, "COPY --from=builder /app/package.json ./")
	fmt.Fprintf(&b, "EXPOSE %d\n", svc.Port)
	fmt.Fprintln(&b, dockerCMD(svc.StartCmd))
	return b.String()
}

func renderReact(svc detector.Service, prefix string) string {
	pm := svc.PackageManager
	if pm == "" {
		pm = "npm"
	}
	install := safeInstallCmd(svc)

	var b strings.Builder
	header(&b)
	fmt.Fprintln(&b, "FROM node:20-alpine AS builder")
	fmt.Fprintln(&b, "WORKDIR /app")
	fmt.Fprintf(&b, "COPY %spackage*.json ./\n", prefix)
	if lock := copyLockfile(prefix, pm, svc.HasLockfile); lock != "" {
		fmt.Fprintln(&b, lock)
	}
	fmt.Fprintf(&b, "RUN %s\n", install)
	fmt.Fprintf(&b, "COPY %s. ./\n", prefix)
	writeBuildEnv(&b, svc)
	fmt.Fprintf(&b, "RUN %s\n", joinArgs(svc.BuildCmd))

	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "FROM nginx:alpine AS runner")
	curlInstall(&b, svc.Framework)
	// Vite emits to dist/, CRA emits to build/. Copy whichever exists; nginx
	// is happy with whatever lands in /usr/share/nginx/html.
	if svc.Framework == "cra" {
		fmt.Fprintln(&b, "COPY --from=builder /app/build /usr/share/nginx/html")
	} else {
		fmt.Fprintln(&b, "COPY --from=builder /app/dist /usr/share/nginx/html")
	}
	fmt.Fprintln(&b, "EXPOSE 80")
	fmt.Fprintln(&b, `CMD ["nginx", "-g", "daemon off;"]`)
	fmt.Fprintf(&b, "# Original dev start: %s (port %d)\n", strings.Join(svc.StartCmd, " "), svc.Port)
	return b.String()
}

func renderNode(svc detector.Service, prefix string) string {
	pm := svc.PackageManager
	if pm == "" {
		pm = "npm"
	}
	install := safeInstallCmd(svc)

	var b strings.Builder
	header(&b)
	fmt.Fprintln(&b, "FROM node:20-alpine")
	fmt.Fprintln(&b, "WORKDIR /app")
	curlInstall(&b, svc.Framework)
	fmt.Fprintf(&b, "COPY %spackage*.json ./\n", prefix)
	if lock := copyLockfile(prefix, pm, svc.HasLockfile); lock != "" {
		fmt.Fprintln(&b, lock)
	}
	fmt.Fprintf(&b, "RUN %s\n", install)
	fmt.Fprintf(&b, "COPY %s. ./\n", prefix)
	writeBuildEnv(&b, svc)
	// Run the project's build step when present (Express/Node apps that
	// compile TS, or SSR frameworks routed here). Harmless when BuildCmd is
	// empty — the RUN is omitted entirely.
	if build := joinArgs(svc.BuildCmd); build != "" {
		fmt.Fprintf(&b, "RUN %s\n", build)
	}
	fmt.Fprintf(&b, "EXPOSE %d\n", svc.Port)
	fmt.Fprintln(&b, dockerCMD(svc.StartCmd))
	return b.String()
}

func renderFastAPI(svc detector.Service, prefix string) string {
	var b strings.Builder
	header(&b)
	fmt.Fprintln(&b, "FROM python:3.12-slim")
	fmt.Fprintln(&b, "WORKDIR /app")
	pythonSystemDeps(&b, svc)
	writePythonDeps(&b, svc, prefix)
	// Guarantee the uvicorn ASGI server is on PATH even when the project's
	// own manifest forgot to pin it (common with bare `fastapi` deps).
	b.WriteString("RUN pip install --no-cache-dir \"uvicorn[standard]\"\n")
	b.WriteString("ENV PYTHONPATH=/app\n")
	fmt.Fprintf(&b, "COPY %s. ./\n", prefix)
	pythonDeferredInstall(&b, svc)
	fmt.Fprintf(&b, "EXPOSE %d\n", svc.Port)
	fmt.Fprintln(&b, dockerCMD(svc.StartCmd))
	return b.String()
}

func renderFlask(svc detector.Service, prefix string) string {
	var b strings.Builder
	header(&b)
	fmt.Fprintln(&b, "FROM python:3.12-slim")
	fmt.Fprintln(&b, "WORKDIR /app")
	pythonSystemDeps(&b, svc)
	writePythonDeps(&b, svc, prefix)
	b.WriteString("ENV PYTHONPATH=/app\n")
	fmt.Fprintf(&b, "COPY %s. ./\n", prefix)
	pythonDeferredInstall(&b, svc)
	fmt.Fprintln(&b, "ENV FLASK_RUN_HOST=0.0.0.0")
	fmt.Fprintf(&b, "EXPOSE %d\n", svc.Port)
	fmt.Fprintln(&b, dockerCMD(svc.StartCmd))
	return b.String()
}

func renderDjango(svc detector.Service, prefix string) string {
	var b strings.Builder
	header(&b)
	fmt.Fprintln(&b, "FROM python:3.12-slim")
	fmt.Fprintln(&b, "WORKDIR /app")
	pythonSystemDeps(&b, svc)
	writePythonDeps(&b, svc, prefix)
	b.WriteString("ENV PYTHONPATH=/app\n")
	fmt.Fprintf(&b, "COPY %s. ./\n", prefix)
	pythonDeferredInstall(&b, svc)
	fmt.Fprintf(&b, "EXPOSE %d\n", svc.Port)
	fmt.Fprintln(&b, dockerCMD(svc.StartCmd))
	return b.String()
}

func renderPython(svc detector.Service, prefix string) string {
	var b strings.Builder
	header(&b)
	fmt.Fprintln(&b, "FROM python:3.12-slim")
	fmt.Fprintln(&b, "WORKDIR /app")
	pythonSystemDeps(&b, svc)
	writePythonDeps(&b, svc, prefix)
	b.WriteString("ENV PYTHONPATH=/app\n")
	fmt.Fprintf(&b, "COPY %s. ./\n", prefix)
	pythonDeferredInstall(&b, svc)
	fmt.Fprintf(&b, "EXPOSE %d\n", svc.Port)
	fmt.Fprintln(&b, dockerCMD(svc.StartCmd))
	return b.String()
}

// renderGo produces a multi-stage Dockerfile for a Go service: build in
// golang:alpine, run in a minimal alpine image with curl for healthchecks.
func renderGo(svc detector.Service, prefix string) string {
	bin := binaryName(svc)
	var b strings.Builder
	header(&b)
	fmt.Fprintln(&b, "FROM golang:1.23-alpine AS builder")
	fmt.Fprintln(&b, "WORKDIR /app")
	fmt.Fprintln(&b, "RUN apk add --no-cache git")
	fmt.Fprintf(&b, "COPY %sgo.mod %sgo.sum* ./\n", prefix, prefix)
	fmt.Fprintln(&b, "RUN go mod download")
	fmt.Fprintf(&b, "COPY %s. ./\n", prefix)
	writeBuildEnv(&b, svc)
	fmt.Fprintf(&b, "RUN %s\n", joinArgs(svc.BuildCmd))
	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "FROM alpine:latest")
	curlInstall(&b, svc.Framework)
	fmt.Fprintln(&b, "WORKDIR /app")
	fmt.Fprintf(&b, "COPY --from=builder /app/%s ./%s\n", bin, bin)
	fmt.Fprintf(&b, "EXPOSE %d\n", svc.Port)
	fmt.Fprintln(&b, dockerCMD(svc.StartCmd))
	return b.String()
}

// renderRust produces a multi-stage Dockerfile for a Rust service: build in
// rust:slim, run in debian:slim with curl for healthchecks.
func renderRust(svc detector.Service, prefix string) string {
	startPath := ""
	if len(svc.StartCmd) > 0 {
		startPath = svc.StartCmd[0]
	}
	binPath := strings.TrimPrefix(startPath, "./")
	if binPath == "" {
		binPath = "target/release/app"
	}
	binDir := ""
	if idx := strings.LastIndex(binPath, "/"); idx > 0 {
		binDir = binPath[:idx]
	}
	var b strings.Builder
	header(&b)
	fmt.Fprintln(&b, "FROM rust:1.83-slim AS builder")
	fmt.Fprintln(&b, "WORKDIR /app")
	fmt.Fprintf(&b, "COPY %sCargo.toml %sCargo.lock* ./\n", prefix, prefix)
	fmt.Fprintf(&b, "COPY %ssrc ./src\n", prefix)
	writeBuildEnv(&b, svc)
	fmt.Fprintf(&b, "RUN %s\n", joinArgs(svc.BuildCmd))
	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "FROM debian:bookworm-slim")
	curlInstall(&b, svc.Framework)
	fmt.Fprintln(&b, "WORKDIR /app")
	if binDir != "" {
		fmt.Fprintf(&b, "RUN mkdir -p %s\n", binDir)
	}
	fmt.Fprintf(&b, "COPY --from=builder /app/%s ./%s\n", binPath, binPath)
	fmt.Fprintf(&b, "EXPOSE %d\n", svc.Port)
	fmt.Fprintln(&b, dockerCMD(svc.StartCmd))
	return b.String()
}

// binaryName extracts the compiled binary name from the service's StartCmd
// (e.g. "./app" → "app"). Falls back to "app" when the start command doesn't
// start with "./".
func binaryName(svc detector.Service) string {
	if len(svc.StartCmd) > 0 && strings.HasPrefix(svc.StartCmd[0], "./") {
		return strings.TrimPrefix(svc.StartCmd[0], "./")
	}
	return "app"
}

// DockerignoreContent returns a comprehensive .dockerignore.
// build context is the repository root (".."), so every `COPY . ./` in the
// generated Dockerfiles would otherwise drag the host's node_modules (with
// macOS/Windows-native binaries), .git, .venv, build artefacts, and the
// generated yoink-outputs directory into the image. That bloats the context,
// slows builds, and — worst of all — overwrites the freshly-installed
// node_modules inside the container with the developer's host binaries,
// crashing the image at runtime (esbuild/sharp "wrong platform").
//
// Excluding .env (but re-including .env.example) also prevents real secrets
// from being baked into an image.
func DockerignoreContent() string {
	return `# Generated by yoink — keeps the build context lean and prevents host
# node_modules / .venv / .git from leaking into images.
# Safe to edit; re-run yoink init --force to regenerate.

# VCS & editor
.git
.gitignore
**/.idea
**/.vscode

# Dependency dirs (host binaries are platform-specific — never copy them)
**/node_modules
.venv
venv
env
**/__pycache__
**/.pytest_cache
**/.mypy_cache
**/.ruff_cache
**/.tox
**/.eggs
**/*.egg-info
**/site-packages

# Build output & framework caches
**/.next
**/.nuxt
**/.turbo
**/.cache
**/dist
**/build
**/coverage
**/.nyc_output
**/target

# Yoink's own generated output (Dockerfile + compose are still readable by
# Docker even when listed here; only COPY into images is blocked).
yoink-outputs

# Logs & OS cruft
*.log
npm-debug.log*
yarn-debug.log*
yarn-error.log*
pnpm-debug.log*
.DS_Store
Thumbs.db

# Real env files must never enter an image; keep the example template.
**/.env
!**/.env.example
`
}

func header(b *strings.Builder) {
	b.WriteString("# syntax=docker/dockerfile:1.6\n")
}

// writeBuildEnv emits ENV directives for the service's BuildEnv map. These
// satisfy build-time env-var validation (e.g. Next.js `next build` evaluates
// API routes that throw if JWT_SECRET/DATABASE_URL aren't set). The values
// are placeholders — at runtime, compose's env_file overrides them with real
// values.
//
// SECRET SAFETY: environment variables whose names look like secrets
// (SECRET, KEY, PASSWORD, TOKEN, CREDENTIAL, PRIVATE) are ALWAYS emitted
// with the generic placeholder "yoink-build-placeholder", never with their
// actual value. This prevents real secrets from being baked into image
// layers. Non-secret configuration (PORT, DATABASE_URL, NODE_ENV, etc.)
// uses the real value from the env model.
func writeBuildEnv(b *strings.Builder, svc detector.Service) {
	if len(svc.BuildEnv) == 0 {
		return
	}
	keys := make([]string, 0, len(svc.BuildEnv))
	for k := range svc.BuildEnv {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := svc.BuildEnv[k]
		if isSecretEnvVar(k) || strings.TrimSpace(v) == "" {
			v = "yoink-build-placeholder"
		}
		fmt.Fprintf(b, "ENV %s=%s\n", k, v)
	}
}

// isSecretEnvVar reports whether the variable name looks like a secret that
// must never be baked into an image layer. The heuristic is intentionally
// broad — false positives (treating a non-secret as a secret) only mean the
// build-time placeholder is used, which is harmless.
func isSecretEnvVar(name string) bool {
	upper := strings.ToUpper(name)
	for _, marker := range []string{"SECRET", "PASSWORD", "TOKEN", "CREDENTIAL", "PRIVATE_KEY", "API_KEY"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}

// curlInstall emits the package-manager command that puts curl on PATH, used
// by the compose-level healthcheck. Skipped if the framework slot is
// unknown.
func curlInstall(b *strings.Builder, framework string) {
	if !healthcheck.RuntimeNeedsCurl(framework) {
		return
	}
	if cmd := healthcheck.InstallCurl(framework); cmd != "" {
		b.WriteString(cmd)
		b.WriteString("\n")
	}
}

// copyLockfile emits a COPY instruction for the project's lockfile. When
// hasLockfile is false (no lockfile detected on disk), returns "" — the
// Dockerfile will fall back to `npm install` via safeInstallCmd.
func copyLockfile(prefix, pm string, hasLockfile bool) string {
	if !hasLockfile {
		return ""
	}
	switch pm {
	case "pnpm":
		return fmt.Sprintf("COPY %spnpm-lock.yaml ./", prefix)
	case "yarn":
		return fmt.Sprintf("COPY %syarn.lock ./", prefix)
	default:
		return fmt.Sprintf("COPY %spackage-lock.json* ./", prefix)
	}
}

// safeInstallCmd returns the install command string for a service. When npm
// is the package manager and no lockfile exists on disk, `npm ci` would fail
// (it requires package-lock.json), so we fall back to `npm install`.
func safeInstallCmd(svc detector.Service) string {
	cmd := joinArgs(svc.InstallCmd)
	if cmd == "" {
		pm := svc.PackageManager
		if pm == "" {
			pm = "npm"
		}
		cmd = joinArgs(detector.PMInstall(pm))
	}
	if svc.PackageManager == "npm" || svc.PackageManager == "" {
		if !svc.HasLockfile {
			return "npm install --no-audit --no-fund"
		}
	}
	return cmd
}

// pythonSystemDeps emits a single apt-get layer that installs curl (for the
// compose healthcheck) plus any system -dev packages the project's
// C-extension dependencies need (psycopg -> libpq-dev, mysqlclient ->
// default-libmysqlclient-dev, lxml -> libxml2-dev, ...). Keeping curl and
// the native headers in one RUN avoids an extra layer and a second
// apt-get update.
func pythonSystemDeps(b *strings.Builder, svc detector.Service) {
	pkgs := []string{"curl"}
	pkgs = append(pkgs, pydeps.AptPackages(svc.PythonDeps)...)
	fmt.Fprintf(b, "RUN apt-get update && apt-get install -y --no-install-recommends %s && rm -rf /var/lib/apt/lists/*\n", strings.Join(pkgs, " "))
}

// writePythonDeps emits the COPY + RUN instructions that install Python
// dependencies from a manifest/lockfile. The install of the project itself
// (`pip install .` / `uv pip install .` for a PEP 621 pyproject) is deferred
// to pythonDeferredInstall because it needs the full source tree, which the
// renderer copies in after this step.
//
// PYTHONPATH=/app is set by the renderer so the app's own modules are
// importable by `uvicorn main:app` regardless of how deps were installed.
func writePythonDeps(b *strings.Builder, svc detector.Service, prefix string) {
	switch {
	case svc.PackageManager == "uv":
		fmt.Fprintf(b, "COPY %suv.lock* %spyproject.toml* %srequirements*.txt* ./\n", prefix, prefix, prefix)
		b.WriteString("RUN pip install --no-cache-dir uv\n")
		if svc.PythonManifest != "pyproject.toml" {
			// requirements.txt installs from the lockfile/manifest only —
			// no source needed, so do it now for layer caching.
			b.WriteString("RUN uv pip install --system --no-cache-dir -r requirements.txt\n")
		}
		// pyproject `uv pip install .` is deferred (needs source).
	case svc.PackageManager == "poetry":
		fmt.Fprintf(b, "COPY %spyproject.toml %spoetry.lock* ./\n", prefix, prefix)
		b.WriteString("RUN pip install --no-cache-dir poetry && POETRY_VIRTUALENVS_CREATE=false poetry install --no-interaction --no-root\n")
	case svc.PythonManifest == "Pipfile":
		fmt.Fprintf(b, "COPY %sPipfile %sPipfile.lock* ./\n", prefix, prefix)
		b.WriteString("RUN pip install --no-cache-dir pipenv && pipenv install --system --deploy\n")
	case svc.PythonManifest == "pyproject.toml":
		// PEP 621: copy the manifest so it's present; `pip install .` runs
		// after the source copy (see pythonDeferredInstall) because the
		// build backend needs the package source to build the wheel.
		fmt.Fprintf(b, "COPY %spyproject.toml ./\n", prefix)
		fmt.Fprintf(b, "COPY %ssetup.py* %ssetup.cfg* ./\n", prefix, prefix)
	default: // requirements.txt (the common case)
		fmt.Fprintf(b, "COPY %srequirements.txt ./\n", prefix)
		b.WriteString("RUN pip install --no-cache-dir -r requirements.txt\n")
	}
}

// pythonDeferredInstall emits the install step that needs the full source
// tree (PEP 621 `pip install .` / uv `uv pip install .`). The renderer calls
// it AFTER `COPY . ./` so the build backend can see the package source.
// Emits nothing for requirements.txt / poetry / pipenv, which already
// installed from a manifest or lockfile in writePythonDeps.
func pythonDeferredInstall(b *strings.Builder, svc detector.Service) {
	switch {
	case svc.PackageManager == "uv" && svc.PythonManifest == "pyproject.toml":
		b.WriteString("RUN uv pip install --system --no-cache-dir .\n")
	case svc.PackageManager != "poetry" && svc.PythonManifest == "pyproject.toml":
		b.WriteString("RUN pip install --no-cache-dir .\n")
	}
}

func joinArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return strings.Join(args, " ")
}

func dockerCMD(args []string) string {
	if len(args) == 0 {
		return `CMD ["sh", "-c", "echo no start command detected; sleep infinity"]`
	}
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = fmt.Sprintf("%q", a)
	}
	return "CMD [" + strings.Join(parts, ", ") + "]"
}
