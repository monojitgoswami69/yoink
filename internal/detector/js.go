package detector

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// detectJS classifies a JavaScript/TypeScript project. It returns false to
// signal that the candidate is not a deployable service (a workspace root, a
// pure library, build tooling, etc.) and should be discarded.
func detectJS(dir string, svc *Service) bool {
	svc.Language = "javascript"
	if exists(filepath.Join(dir, "tsconfig.json")) {
		svc.Language = "typescript"
		svc.addEvidence("language=typescript", "tsconfig.json exists", "strong")
	} else {
		svc.addEvidence("language=javascript", "no tsconfig.json", "medium")
	}
	svc.PackageManager = detectJSPM(dir)
	svc.HasLockfile = HasLockfile(dir, svc.PackageManager)
	svc.addEvidence("pm="+svc.PackageManager, "lockfile detection", "strong")
	if svc.HasLockfile {
		svc.addEvidence("has_lockfile=true", "lockfile present on disk", "strong")
	}

	pkg, ok := readPackageJSON(dir)
	if !ok {
		return false
	}

	// Workspace root package.json (no deployable of its own).
	if len(pkg.Workspaces) > 0 {
		return false
	}

	deps := normaliseDeps(pkg.Dependencies, pkg.DevDependencies)
	framework := pickJSFramework(deps)

	hasStart := pkg.Scripts["start"] != "" || pkg.Scripts["dev"] != ""
	// Library / build tooling: no recognised framework and no start script.
	if framework == "" && !hasStart {
		return false
	}
	// Adversarial: for config-file frameworks (next, nuxt, vite), a dep
	// alone without any start script AND without a config file is likely a
	// library that depends on the framework, not an application built with it.
	// For other frameworks (express, fastify, node), the dep is sufficient
	// evidence — they don't have config files to require.
	if framework != "" && !hasStart && isConfigFramework(framework) {
		if !hasFrameworkConfig(dir, framework) {
			return false
		}
	}

	applyJSFramework(svc, framework, pkg.Main, pkg.Scripts)
	svc.addEvidence("framework="+framework, "package.json dependency", "strong")
	svc.InstallCmd = PMInstall(svc.PackageManager)

	// Refine the port from the entrypoint if we can. Patterns like
	//   process.env.PORT || 4000
	//   listen(parseInt(process.env.PORT || '4000'))
	// are common in plain Node services.
	if port, ok := portFromJSEntry(dir, pkg.Main, pkg.Scripts); ok {
		svc.Port = port
		svc.addEvidence(fmt.Sprintf("port=%d", port), "entry file process.env.PORT or .listen()", "strong")
	}

	// Directory hint: a frontend framework served from apps/api/ is almost
	// certainly an API route folder, not a deployable frontend.
	low := strings.ToLower(svc.Directory)
	if svc.Type == "frontend" && (strings.Contains(low, "/api") || strings.HasSuffix(low, "api") || strings.Contains(low, "server")) {
		svc.Type = "backend"
		svc.addEvidence("type=backend", "directory hint (api/server)", "medium")
	}
	return true
}

type packageJSON struct {
	Workspaces      json.RawMessage   `json:"workspaces"`
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Main            string            `json:"main"`
}

func readPackageJSON(dir string) (packageJSON, bool) {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return packageJSON{}, false
	}
	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return packageJSON{}, false
	}
	return pkg, true
}

func normaliseDeps(maps ...map[string]string) map[string]bool {
	deps := map[string]bool{}
	for _, m := range maps {
		for k := range m {
			deps[strings.ToLower(k)] = true
		}
	}
	return deps
}

func pickJSFramework(deps map[string]bool) string {
	switch {
	case deps["next"]:
		return "next"
	case deps["@nestjs/core"]:
		return "nest"
	case deps["nuxt"]:
		return "nuxt"
	case deps["astro"]:
		return "astro"
	case deps["@sveltejs/kit"]:
		return "sveltekit"
	case deps["@remix-run/react"], deps["@remix-run/node"], deps["@remix-run/dev"]:
		return "remix"
	case deps["fastify"]:
		return "fastify"
	case deps["express"], deps["koa"], deps["hapi"]:
		return "express"
	case deps["vite"]:
		return "vite"
	case deps["react-scripts"]:
		return "cra"
	case deps["react"]:
		return "react"
	}
	return ""
}

// applyJSFramework fills in framework-specific fields. An empty framework
// means "no recognised framework but a start script exists" — treated as a
// plain Node service.
func applyJSFramework(svc *Service, framework, main string, scripts map[string]string) {
	pm := svc.PackageManager
	switch framework {
	case "next":
		svc.Framework, svc.Type, svc.Confidence, svc.Port = "next", "frontend", "high", 3000
		svc.BuildCmd, svc.StartCmd = pmRun(pm, "build"), pmRun(pm, "start")
	case "nest":
		svc.Framework, svc.Type, svc.Confidence, svc.Port = "nest", "backend", "high", 3000
		svc.BuildCmd, svc.StartCmd = pmRun(pm, "build"), pmRun(pm, "start:prod")
	case "nuxt":
		// Nuxt 3 produces a self-contained .output/ node server; `npm run
		// start` runs `node .output/server/index.mjs`.
		svc.Framework, svc.Type, svc.Confidence, svc.Port = "nuxt", "frontend", "high", 3000
		svc.BuildCmd, svc.StartCmd = pmRun(pm, "build"), pmRun(pm, "start")
	case "astro":
		// Astro static output (dist/) is served by nginx in the generator.
		svc.Framework, svc.Type, svc.Confidence, svc.Port = "astro", "frontend", "high", 4321
		svc.BuildCmd = pmRun(pm, "build")
	case "sveltekit":
		// adapter-node produces build/ run via the package's start script.
		svc.Framework, svc.Type, svc.Confidence, svc.Port = "sveltekit", "backend", "medium", 3000
		svc.BuildCmd, svc.StartCmd = pmRun(pm, "build"), nodeStart(main, scripts)
	case "remix":
		svc.Framework, svc.Type, svc.Confidence, svc.Port = "remix", "backend", "high", 3000
		svc.BuildCmd, svc.StartCmd = pmRun(pm, "build"), nodeStart(main, scripts)
	case "fastify":
		svc.Framework, svc.Type, svc.Confidence, svc.Port = "fastify", "backend", "high", 3000
		svc.StartCmd = nodeStart(main, scripts)
	case "express":
		svc.Framework, svc.Type, svc.Confidence, svc.Port = "express", "backend", "high", 3000
		svc.StartCmd = nodeStart(main, scripts)
	case "vite":
		svc.Framework, svc.Type, svc.Confidence, svc.Port = "vite", "frontend", "high", 5173
		svc.BuildCmd, svc.StartCmd = pmRun(pm, "build"), pmRun(pm, "preview")
	case "cra":
		svc.Framework, svc.Type, svc.Confidence, svc.Port = "cra", "frontend", "high", 3000
		svc.BuildCmd, svc.StartCmd = pmRun(pm, "build"), pmRun(pm, "start")
	case "react":
		svc.Framework, svc.Type, svc.Confidence, svc.Port = "react", "frontend", "medium", 3000
		svc.BuildCmd, svc.StartCmd = pmRun(pm, "build"), pmRun(pm, "start")
	default:
		svc.Framework, svc.Type, svc.Confidence, svc.Port = "node", "backend", "low", 3000
		if scripts["build"] != "" {
			svc.BuildCmd = pmRun(pm, "build")
		}
		svc.StartCmd = nodeStart(main, scripts)
	}
}

func nodeStart(main string, scripts map[string]string) []string {
	if _, ok := scripts["start"]; ok {
		return []string{"npm", "start"}
	}
	if main != "" {
		return []string{"node", main}
	}
	return []string{"node", "index.js"}
}

var (
	jsPortRe   = regexp.MustCompile(`(?:process\.env\.PORT|PORT)[\s)|]*\|\|\s*['"]?(\d{2,5})['"]?`)
	jsListenRe = regexp.MustCompile(`\.listen\(\s*['"]?(\d{2,5})['"]?`)
)

// portFromJSEntry scans likely entry files for a port literal. Returns
// (port, true) on a hit.
func portFromJSEntry(dir, main string, scripts map[string]string) (int, bool) {
	candidates := jsEntryCandidates(main, scripts)
	seen := map[string]bool{}
	for _, c := range candidates {
		if seen[c] {
			continue
		}
		seen[c] = true
		data, err := os.ReadFile(filepath.Join(dir, c))
		if err != nil {
			continue
		}
		text := string(data)
		for _, re := range []*regexp.Regexp{jsPortRe, jsListenRe} {
			if m := re.FindStringSubmatch(text); m != nil {
				if n, err := strconv.Atoi(m[1]); err == nil && n >= 80 && n <= 65535 {
					return n, true
				}
			}
		}
	}
	return 0, false
}

func jsEntryCandidates(main string, scripts map[string]string) []string {
	out := make([]string, 0, 8)
	if main != "" {
		out = append(out, main)
	}
	out = append(out, "server.js", "index.js", "app.js", "src/server.ts", "src/index.ts", "server.ts", "index.ts")
	if s, ok := scripts["start"]; ok {
		for _, f := range strings.Fields(s) {
			if strings.HasSuffix(f, ".js") || strings.HasSuffix(f, ".ts") || strings.HasSuffix(f, ".mjs") {
				out = append(out, f)
			}
		}
	}
	return out
}

func detectJSPM(dir string) string {
	switch {
	case exists(filepath.Join(dir, "bun.lock")):
		return "bun"
	case exists(filepath.Join(dir, "bun.lockb")):
		return "bun"
	case exists(filepath.Join(dir, "pnpm-lock.yaml")):
		return "pnpm"
	case exists(filepath.Join(dir, "yarn.lock")):
		return "yarn"
	}
	return "npm"
}

func PMInstall(pm string) []string {
	switch pm {
	case "pnpm":
		return []string{"pnpm", "install", "--frozen-lockfile"}
	case "yarn":
		return []string{"yarn", "install", "--frozen-lockfile"}
	case "bun":
		return []string{"bun", "install", "--frozen-lockfile"}
	}
	return []string{"npm", "ci", "--no-audit", "--no-fund"}
}

// HasLockfile reports whether the project directory contains a lockfile for
// the detected package manager. The generator uses this to decide between
// `npm ci` (needs lockfile) and `npm install` (fallback).
func HasLockfile(dir, pm string) bool {
	switch pm {
	case "pnpm":
		return exists(filepath.Join(dir, "pnpm-lock.yaml"))
	case "yarn":
		return exists(filepath.Join(dir, "yarn.lock"))
	case "bun":
		return exists(filepath.Join(dir, "bun.lock")) || exists(filepath.Join(dir, "bun.lockb"))
	default:
		return exists(filepath.Join(dir, "package-lock.json"))
	}
}

func pmRun(pm, script string) []string {
	switch pm {
	case "pnpm":
		return []string{"pnpm", "run", script}
	case "yarn":
		return []string{"yarn", script}
	}
	return []string{"npm", "run", script}
}

// hasFrameworkConfig checks for framework-specific config files that provide
// additional evidence beyond a bare dependency. Used to prevent false
// positives (e.g. a library that depends on "next" but has no next.config).
func hasFrameworkConfig(dir, framework string) bool {
	switch framework {
	case "next":
		for _, name := range []string{"next.config.js", "next.config.mjs", "next.config.ts"} {
			if exists(filepath.Join(dir, name)) {
				return true
			}
		}
	case "nuxt":
		for _, name := range []string{"nuxt.config.ts", "nuxt.config.js"} {
			if exists(filepath.Join(dir, name)) {
				return true
			}
		}
	case "vite":
		for _, name := range []string{"vite.config.ts", "vite.config.js", "vite.config.mjs"} {
			if exists(filepath.Join(dir, name)) {
				return true
			}
		}
	}
	return false
}

// isConfigFramework reports whether the framework has a well-known config
// file that can serve as additional evidence. Only these frameworks require
// script+config evidence; others (express, fastify, node) are kept on dep
// evidence alone.
func isConfigFramework(framework string) bool {
	switch framework {
	case "next", "nuxt", "vite":
		return true
	}
	return false
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
