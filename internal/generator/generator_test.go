package generator

import (
	"strings"
	"testing"

	"yoink/internal/detector"
	"yoink/internal/infra"

	"gopkg.in/yaml.v3"
)

// deterministicPort returns a portFn that hands out the preferred port the
// first time it's seen and bumps by 1 for each subsequent claim — matching
// the previous, non-probing behaviour so existing assertions stay
// meaningful.
func deterministicPort() func(int) int {
	used := map[int]bool{}
	return func(p int) int {
		for used[p] {
			p++
		}
		used[p] = true
		return p
	}
}

func TestBuildEmitsDockerfilePerServiceAndCompose(t *testing.T) {
	services := []detector.Service{
		{ID: "service-1", Type: "frontend", Directory: "apps/web", Language: "typescript", Framework: "next", PackageManager: "npm", Port: 3000, InstallCmd: []string{"npm", "ci"}, BuildCmd: []string{"npm", "run", "build"}, StartCmd: []string{"npm", "start"}},
		{ID: "service-2", Type: "backend", Directory: "services/api", Language: "python", Framework: "fastapi", PackageManager: "pip", Port: 8000, InstallCmd: []string{"pip", "install", "--no-cache-dir", "-r", "requirements.txt"}, StartCmd: []string{"uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"}},
	}
	out := Build(services, Options{Repo: "myrepo", OutputSubdir: "yoink-outputs", PortFn: deterministicPort()})

	if _, ok := out.Files["Dockerfile.service-1"]; !ok {
		t.Errorf("missing Dockerfile.service-1; files=%v", keys(out.Files))
	}
	if _, ok := out.Files["Dockerfile.service-2"]; !ok {
		t.Errorf("missing Dockerfile.service-2")
	}
	compose, ok := out.Files["docker-compose.yml"]
	if !ok {
		t.Fatal("missing docker-compose.yml")
	}
	if !strings.Contains(compose, "context: ..") {
		t.Errorf("compose context should be `..` (repo root); got:\n%s", compose)
	}
	if !strings.Contains(compose, "dockerfile: yoink-outputs/Dockerfile.service-1") {
		t.Errorf("compose dockerfile path wrong:\n%s", compose)
	}
	if !strings.Contains(compose, "env-vars/service-1/.env") {
		t.Errorf("compose env_file path wrong:\n%s", compose)
	}
	if strings.Count(compose, "\"3000:3000\"") > 1 {
		t.Errorf("host port collision in compose:\n%s", compose)
	}
	if !strings.Contains(compose, "healthcheck:") {
		t.Errorf("compose should declare healthchecks; got:\n%s", compose)
	}
}

func TestDockerfileForNextInstallsCurlAndUsesRepoRootContext(t *testing.T) {
	svc := detector.Service{ID: "s1", Directory: "apps/web", Framework: "next", PackageManager: "npm", Port: 3000, InstallCmd: []string{"npm", "ci"}, BuildCmd: []string{"npm", "run", "build"}, StartCmd: []string{"npm", "start"}}
	df := renderDockerfile(svc)
	if !strings.Contains(df, "COPY apps/web/package*.json ./") {
		t.Errorf("dockerfile should COPY from apps/web/:\n%s", df)
	}
	if !strings.Contains(df, "EXPOSE 3000") {
		t.Errorf("dockerfile should EXPOSE 3000:\n%s", df)
	}
	if !strings.Contains(df, `CMD ["npm", "start"]`) {
		t.Errorf("dockerfile should have CMD npm start; got:\n%s", df)
	}
	if !strings.Contains(df, "apk add --no-cache curl") {
		t.Errorf("dockerfile should install curl for healthchecks; got:\n%s", df)
	}
}

func TestDockerfileForFastAPIInstallsCurlViaApt(t *testing.T) {
	svc := detector.Service{ID: "s1", Framework: "fastapi", Language: "python", Port: 8000, InstallCmd: []string{"pip", "install", "-r", "requirements.txt"}, StartCmd: []string{"uvicorn", "main:app"}}
	df := renderDockerfile(svc)
	if !strings.Contains(df, "apt-get install -y --no-install-recommends curl") {
		t.Errorf("fastapi dockerfile should install curl via apt; got:\n%s", df)
	}
}

func TestComposeHandlesPortCollisions(t *testing.T) {
	svcs := []detector.Service{
		{ID: "a", Framework: "express", Port: 3000},
		{ID: "b", Framework: "node", Port: 3000},
		{ID: "c", Framework: "fastify", Port: 3000},
	}
	out := Build(svcs, Options{Repo: "repo", OutputSubdir: "yoink-outputs", PortFn: deterministicPort()})
	c := out.Files["docker-compose.yml"]
	if !strings.Contains(c, "\"3000:3000\"") {
		t.Errorf("first service should bind 3000:3000")
	}
	if !strings.Contains(c, "\"3001:3000\"") {
		t.Errorf("second service should remap 3001:3000")
	}
	if !strings.Contains(c, "\"3002:3000\"") {
		t.Errorf("third service should remap 3002:3000")
	}
}

func TestBuildOverridesPortForStaticSPA(t *testing.T) {
	svcs := []detector.Service{
		{ID: "a", Framework: "vite", Port: 5173},
		{ID: "b", Framework: "cra", Port: 3000},
	}
	out := Build(svcs, Options{Repo: "repo", OutputSubdir: "yoink-outputs", PortFn: deterministicPort()})
	c := out.Files["docker-compose.yml"]
	if !strings.Contains(c, "\"80:80\"") {
		t.Errorf("first static-SPA should bind 80:80 (nginx). Got:\n%s", c)
	}
	if !strings.Contains(c, "\"81:80\"") {
		t.Errorf("second static-SPA should remap to 81:80. Got:\n%s", c)
	}
}

func TestComposeEmitsInfraServicesAndDependsOn(t *testing.T) {
	svcs := []detector.Service{
		{ID: "service-1", Framework: "fastapi", Directory: "api", Port: 8000, Type: "backend"},
	}
	inf := []infra.Service{
		{
			Kind: infra.KindPostgres, Name: "postgres", Image: "postgres:16-alpine", Port: 5432,
			Env:        map[string]string{"POSTGRES_DB": "app"},
			VolumeName: "yoink-postgres-data", VolumePath: "/var/lib/postgresql/data",
			Healthcheck: infra.Healthcheck{Test: []string{"CMD-SHELL", "pg_isready"}, Interval: "5s", Timeout: "3s", Retries: 10},
		},
	}
	links := map[string][]infra.AppLink{
		"service-1": {{ServiceName: "postgres", EnvVars: map[string]string{"DATABASE_URL": "postgres://"}}},
	}
	out := Build(svcs, Options{Repo: "repo", OutputSubdir: "yoink-outputs", Infra: inf, Links: links, PortFn: deterministicPort()})
	c := out.Files["docker-compose.yml"]

	if !strings.Contains(c, "  postgres:") {
		t.Errorf("compose missing postgres service:\n%s", c)
	}
	if !strings.Contains(c, "image: postgres:16-alpine") {
		t.Errorf("postgres image missing:\n%s", c)
	}
	if !strings.Contains(c, "depends_on:") || !strings.Contains(c, "condition: service_healthy") {
		t.Errorf("expected depends_on with healthy condition:\n%s", c)
	}
	if !strings.Contains(c, "yoink-postgres-data:/var/lib/postgresql/data") {
		t.Errorf("expected named volume mount:\n%s", c)
	}
	if !strings.Contains(c, "volumes:\n  yoink-postgres-data:") {
		t.Errorf("expected top-level volumes block:\n%s", c)
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestComposeYAMLParses(t *testing.T) {
	svcs := []detector.Service{
		{ID: "service-1", Framework: "next", Directory: "apps/web", Port: 3000, Type: "frontend"},
		{ID: "service-2", Framework: "fastapi", Directory: "services/api", Port: 8000, Type: "backend"},
	}
	inf := []infra.Service{
		{
			Kind: infra.KindRedis, Name: "redis", Image: "redis:7-alpine", Port: 6379,
			Healthcheck: infra.Healthcheck{Test: []string{"CMD", "redis-cli", "ping"}, Interval: "5s", Timeout: "3s", Retries: 5},
		},
	}
	out := Build(svcs, Options{Repo: "MyRepo Name!", OutputSubdir: "yoink-outputs", Infra: inf, PortFn: deterministicPort()})

	var parsed struct {
		Services map[string]struct {
			Image string `yaml:"image"`
			Build struct {
				Context    string `yaml:"context"`
				Dockerfile string `yaml:"dockerfile"`
			} `yaml:"build"`
			ContainerName string   `yaml:"container_name"`
			Ports         []string `yaml:"ports"`
			EnvFile       []string `yaml:"env_file"`
			Networks      []string `yaml:"networks"`
			Healthcheck   struct {
				Test interface{} `yaml:"test"`
			} `yaml:"healthcheck"`
		} `yaml:"services"`
		Networks map[string]struct {
			Driver string `yaml:"driver"`
		} `yaml:"networks"`
		Volumes map[string]interface{} `yaml:"volumes"`
	}
	if err := yaml.Unmarshal([]byte(out.Files["docker-compose.yml"]), &parsed); err != nil {
		t.Fatalf("compose file is not valid YAML: %v\n%s", err, out.Files["docker-compose.yml"])
	}
	if len(parsed.Services) != 3 {
		t.Errorf("expected 3 services (2 app + 1 infra) in YAML, got %d", len(parsed.Services))
	}
	if _, ok := parsed.Services["redis"]; !ok {
		t.Errorf("expected redis service entry")
	}
	for _, s := range parsed.Services {
		if strings.ContainsAny(s.ContainerName, " !") {
			t.Errorf("container_name contains illegal chars: %q", s.ContainerName)
		}
		if s.Healthcheck.Test == nil {
			t.Errorf("service %q missing healthcheck", s.ContainerName)
		}
	}
}

// TestBuildEmitsDockerignore guarantees a .dockerignore is generated so the
// host's node_modules/.venv/.git can't leak into images via `COPY . ./`.
func TestBuildEmitsDockerignore(t *testing.T) {
	out := Build(nil, Options{Repo: "r", OutputSubdir: "yoink-outputs"})
	if out.Dockerignore == "" {
		t.Fatal("Build must populate Output.Dockerignore")
	}
	for _, must := range []string{"**/node_modules", ".git", "yoink-outputs", ".venv"} {
		if !strings.Contains(out.Dockerignore, must) {
			t.Errorf("dockerignore missing %q", must)
		}
	}
	// Real .env must be excluded, the example template must survive.
	if !strings.Contains(out.Dockerignore, "**/.env") {
		t.Errorf("dockerignore must exclude **/.env")
	}
	if !strings.Contains(out.Dockerignore, "!**/.env.example") {
		t.Errorf("dockerignore must re-include .env.example")
	}
}

// TestFastAPIGuaranteesUvicornAndPYTHONPATH ensures the FastAPI Dockerfile
// installs uvicorn (even when the project's own manifest forgot it) and sets
// PYTHONPATH so `uvicorn main:app` can import the app from /app.
func TestFastAPIGuaranteesUvicornAndPYTHONPATH(t *testing.T) {
	svc := detector.Service{ID: "s1", Framework: "fastapi", Language: "python", PackageManager: "pip", PythonManifest: "requirements.txt", Port: 8000, StartCmd: []string{"uvicorn", "main:app"}}
	df := renderDockerfile(svc)
	if !strings.Contains(df, "uvicorn[standard]") {
		t.Errorf("fastapi dockerfile must guarantee uvicorn install; got:\n%s", df)
	}
	if !strings.Contains(df, "ENV PYTHONPATH=/app") {
		t.Errorf("fastapi dockerfile must set PYTHONPATH=/app; got:\n%s", df)
	}
}

// TestPythonRenderersSetPYTHONPATH verifies every Python renderer puts the app
// dir on sys.path so module imports work no matter how deps were installed.
func TestPythonRenderersSetPYTHONPATH(t *testing.T) {
	cases := []struct {
		name string
		svc  detector.Service
	}{
		{"flask", detector.Service{ID: "s1", Framework: "flask", Language: "python", PackageManager: "pip", PythonManifest: "requirements.txt", Port: 5000, StartCmd: []string{"flask", "run"}}},
		{"django", detector.Service{ID: "s1", Framework: "django", Language: "python", PackageManager: "pip", PythonManifest: "requirements.txt", Port: 8000, StartCmd: []string{"python", "manage.py", "runserver"}}},
		{"generic-python", detector.Service{ID: "s1", Framework: "python", Language: "python", PackageManager: "pip", PythonManifest: "requirements.txt", Port: 8000, StartCmd: []string{"python", "main.py"}}},
	}
	for _, c := range cases {
		if df := renderDockerfile(c.svc); !strings.Contains(df, "ENV PYTHONPATH=/app") {
			t.Errorf("%s dockerfile must set PYTHONPATH=/app; got:\n%s", c.name, df)
		}
	}
}

// TestPoetryProjectUsesPoetryInstall covers the broken case Qwen identified:
// a pyproject.toml + poetry.lock project must NOT use `pip install .` (which
// fails because poetry's pyproject has no PEP 621 build backend).
func TestPoetryProjectUsesPoetryInstall(t *testing.T) {
	svc := detector.Service{ID: "s1", Framework: "fastapi", Language: "python", PackageManager: "poetry", PythonManifest: "pyproject.toml", Port: 8000, StartCmd: []string{"uvicorn", "main:app"}}
	df := renderDockerfile(svc)
	if !strings.Contains(df, "poetry.lock") {
		t.Errorf("poetry dockerfile must COPY poetry.lock; got:\n%s", df)
	}
	if !strings.Contains(df, "pip install --no-cache-dir poetry") {
		t.Errorf("poetry dockerfile must install poetry; got:\n%s", df)
	}
	if !strings.Contains(df, "POETRY_VIRTUALENVS_CREATE=false poetry install") {
		t.Errorf("poetry dockerfile must install via poetry with virtualenvs disabled; got:\n%s", df)
	}
	// Must NOT use the PEP 621 `pip install .` path which has no build backend.
	if strings.Contains(df, "pip install --no-cache-dir .\n") {
		t.Errorf("poetry dockerfile must not fall back to `pip install .`; got:\n%s", df)
	}
}

// TestPyprojectWithoutPoetryUsesPipInstall covers the PEP 621 path: a
// pyproject.toml project that is NOT poetry should use `pip install .`.
func TestPyprojectWithoutPoetryUsesPipInstall(t *testing.T) {
	svc := detector.Service{ID: "s1", Framework: "fastapi", Language: "python", PackageManager: "pip", PythonManifest: "pyproject.toml", Port: 8000, StartCmd: []string{"uvicorn", "main:app"}}
	df := renderDockerfile(svc)
	if !strings.Contains(df, "pip install --no-cache-dir .") {
		t.Errorf("PEP 621 pyproject dockerfile must use `pip install .`; got:\n%s", df)
	}
}

// TestPyprojectInstallDeferredUntilAfterSource guards the ordering bug where
// `pip install .` ran before the package source was copied (so the build
// backend couldn't see the `app/` package). The install must come AFTER the
// `COPY . ./` line.
func TestPyprojectInstallDeferredUntilAfterSource(t *testing.T) {
	svc := detector.Service{ID: "s1", Framework: "fastapi", Language: "python", PackageManager: "pip", PythonManifest: "pyproject.toml", Port: 8000, StartCmd: []string{"uvicorn", "main:app"}}
	df := renderDockerfile(svc)
	copyIdx := strings.Index(df, "COPY . ./")
	installIdx := strings.Index(df, "pip install --no-cache-dir .")
	if copyIdx < 0 || installIdx < 0 {
		t.Fatalf("missing COPY or pip install; got:\n%s", df)
	}
	if installIdx < copyIdx {
		t.Errorf("pip install . must run AFTER `COPY . ./` (install@%d < copy@%d)", installIdx, copyIdx)
	}
}

// TestRequirementsInstallBeforeSource verifies the common requirements.txt
// path still installs deps BEFORE the source copy (layer caching win) and
// does NOT emit a deferred install.
func TestRequirementsInstallBeforeSource(t *testing.T) {
	svc := detector.Service{ID: "s1", Framework: "fastapi", Language: "python", PackageManager: "pip", PythonManifest: "requirements.txt", Port: 8000, StartCmd: []string{"uvicorn", "main:app"}}
	df := renderDockerfile(svc)
	installIdx := strings.Index(df, "pip install --no-cache-dir -r requirements.txt")
	copyIdx := strings.Index(df, "COPY . ./")
	if installIdx < 0 || copyIdx < 0 {
		t.Fatalf("missing requirements install or COPY; got:\n%s", df)
	}
	if installIdx > copyIdx {
		t.Errorf("requirements install should run BEFORE `COPY . ./`; got:\n%s", df)
	}
}

// TestFastAPIInjectsNativeAptForPsycopg ensures the generator pulls libpq-dev
// when the project declares psycopg, so the C wheel build never fails for
// lack of PostgreSQL headers.
func TestFastAPIInjectsNativeAptForPsycopg(t *testing.T) {
	svc := detector.Service{ID: "s1", Framework: "fastapi", Language: "python", PackageManager: "pip", PythonManifest: "requirements.txt", PythonDeps: []string{"fastapi", "psycopg", "uvicorn"}, Port: 8000, StartCmd: []string{"uvicorn", "main:app"}}
	df := renderDockerfile(svc)
	if !strings.Contains(df, "libpq-dev") {
		t.Errorf("psycopg project must install libpq-dev; got:\n%s", df)
	}
	if !strings.Contains(df, "build-essential") {
		t.Errorf("native dep must install build-essential; got:\n%s", df)
	}
}

// TestFastAPINoNativeAptForPurePython verifies pure-Python projects don't
// bloat the image with build-essential.
func TestFastAPINoNativeAptForPurePython(t *testing.T) {
	svc := detector.Service{ID: "s1", Framework: "fastapi", Language: "python", PackageManager: "pip", PythonManifest: "requirements.txt", PythonDeps: []string{"fastapi", "uvicorn", "pydantic"}, Port: 8000, StartCmd: []string{"uvicorn", "main:app"}}
	df := renderDockerfile(svc)
	if strings.Contains(df, "build-essential") {
		t.Errorf("pure-python project must not install build-essential; got:\n%s", df)
	}
}

// TestPipfileProjectUsesPipenv covers the Pipfile path.
func TestPipfileProjectUsesPipenv(t *testing.T) {
	svc := detector.Service{ID: "s1", Framework: "flask", Language: "python", PackageManager: "pip", PythonManifest: "Pipfile", Port: 5000, StartCmd: []string{"flask", "run"}}
	df := renderDockerfile(svc)
	if !strings.Contains(df, "Pipfile") {
		t.Errorf("pipfile dockerfile must COPY Pipfile; got:\n%s", df)
	}
	if !strings.Contains(df, "pipenv install --system --deploy") {
		t.Errorf("pipfile dockerfile must use pipenv install; got:\n%s", df)
	}
}

// TestNpmInstallsFallbackWhenNoLockfile verifies the generator drops `npm ci`
// in favour of `npm install` when there's no package-lock.json, so a
// lockfile-less repo still builds.
func TestNpmInstallsFallbackWhenNoLockfile(t *testing.T) {
	svc := detector.Service{ID: "s1", Framework: "express", PackageManager: "npm", HasLockfile: false, Port: 3000, StartCmd: []string{"npm", "start"}}
	df := renderDockerfile(svc)
	if !strings.Contains(df, "npm install --no-audit --no-fund") {
		t.Errorf("lockfile-less npm project must use `npm install`; got:\n%s", df)
	}
	if strings.Contains(df, "npm ci") {
		t.Errorf("lockfile-less npm project must NOT use `npm ci`; got:\n%s", df)
	}
}

// TestNextStagesNextConfigSafely ensures the Next.js Dockerfile never emits a
// bare `COPY --from=builder /app/next.config.* ./` or `/app/public` which
// BuildKit rejects when the file/dir is absent. Instead it stages through
// /staged.
func TestNextStagesNextConfigSafely(t *testing.T) {
	svc := detector.Service{ID: "s1", Framework: "next", PackageManager: "npm", HasLockfile: true, Port: 3000, BuildCmd: []string{"npm", "run", "build"}, StartCmd: []string{"npm", "start"}}
	df := renderDockerfile(svc)
	if strings.Contains(df, "COPY --from=builder /app/next.config.* ./") {
		t.Errorf("next dockerfile must not use a bare glob COPY for next.config (fails when absent); got:\n%s", df)
	}
	if strings.Contains(df, "COPY --from=builder /app/public ./public") {
		t.Errorf("next dockerfile must not COPY /app/public directly (fails when absent); got:\n%s", df)
	}
	if !strings.Contains(df, "/staged/cfg") || !strings.Contains(df, "/staged/pub") {
		t.Errorf("next dockerfile must stage config+public through /staged; got:\n%s", df)
	}
	if !strings.Contains(df, "touch /staged/cfg/.keep") {
		t.Errorf("next dockerfile must guarantee /staged/cfg is non-empty (.keep); got:\n%s", df)
	}
}
