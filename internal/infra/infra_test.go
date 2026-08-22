package infra

import (
	"strings"
	"testing"

	"yoink/internal/envvar"
)

func TestInferDetectsPostgresFromDatabaseURL(t *testing.T) {
	res := []envvar.Result{{
		ServiceID:  "service-1",
		Directory:  "apps/api",
		Technology: "fastapi",
		Vars:       []envvar.EnvVar{{Name: "DATABASE_URL"}},
	}}
	inf := Infer(res)
	if len(inf.Services) != 1 || inf.Services[0].Kind != KindPostgres {
		t.Fatalf("expected one postgres service, got %+v", inf.Services)
	}
	links, ok := inf.Links["service-1"]
	if !ok || len(links) != 1 {
		t.Fatalf("expected one link for service-1, got %+v", inf.Links)
	}
	if !strings.Contains(links[0].EnvVars["DATABASE_URL"], "postgres:5432") {
		t.Errorf("expected DATABASE_URL pointing at postgres service, got %q", links[0].EnvVars["DATABASE_URL"])
	}
}

func TestInferDeduplicatesAcrossServices(t *testing.T) {
	res := []envvar.Result{
		{ServiceID: "a", Vars: []envvar.EnvVar{{Name: "REDIS_URL"}, {Name: "DATABASE_URL"}}},
		{ServiceID: "b", Vars: []envvar.EnvVar{{Name: "REDIS_URL"}}},
	}
	inf := Infer(res)
	if len(inf.Services) != 2 {
		t.Fatalf("expected 2 distinct infra services (postgres + redis), got %d (%+v)", len(inf.Services), inf.Services)
	}
	if len(inf.Links["b"]) != 1 || inf.Links["b"][0].ServiceName != "redis" {
		t.Errorf("service b should link only to redis, got %+v", inf.Links["b"])
	}
}

func TestInferIgnoresUnrelatedVars(t *testing.T) {
	res := []envvar.Result{{ServiceID: "x", Vars: []envvar.EnvVar{{Name: "PORT"}, {Name: "API_URL"}}}}
	inf := Infer(res)
	if len(inf.Services) != 0 {
		t.Errorf("expected no infra inferred, got %+v", inf.Services)
	}
}

func TestInferReadsEnvContent(t *testing.T) {
	res := []envvar.Result{{
		ServiceID:  "x",
		EnvContent: "MONGO_URI=mongodb://mongo:27017/app\n",
	}}
	inf := Infer(res)
	if len(inf.Services) != 1 || inf.Services[0].Kind != KindMongo {
		t.Fatalf("expected mongo inferred from env content, got %+v", inf.Services)
	}
}

// At init time, EnrichEnvContent only sees Yoink-generated content (template
// from getCommonVars or the LLM-rewritten template). Existing connection-
// string keys typically carry the framework-default placeholder hostname
// (e.g. `db:5432`); when infra provisions the real service, the inferred
// connection string must win. User edits land at `up` time via state.MergedEnv,
// not here.
func TestEnrichEnvContentOverwritesConnectionStrings(t *testing.T) {
	existing := "DATABASE_URL=postgresql://user:pass@db:5432/app\nSECRET_KEY=abc\n"
	enriched := EnrichEnvContent(existing, map[string]string{
		"DATABASE_URL":      "postgresql://app:app@postgres:5432/app",
		"POSTGRES_PASSWORD": "app",
	})
	if !strings.Contains(enriched, "DATABASE_URL=postgresql://app:app@postgres:5432/app") {
		t.Errorf("DATABASE_URL was not rewritten to point at the inferred postgres service:\n%s", enriched)
	}
	if !strings.Contains(enriched, "SECRET_KEY=abc") {
		t.Errorf("SECRET_KEY (non-connection-string) was clobbered:\n%s", enriched)
	}
	if !strings.Contains(enriched, "POSTGRES_PASSWORD=app") {
		t.Errorf("expected POSTGRES_PASSWORD appended:\n%s", enriched)
	}
	if c := strings.Count(enriched, "DATABASE_URL="); c != 1 {
		t.Errorf("DATABASE_URL appeared %d times (expected 1):\n%s", c, enriched)
	}
}

// Non-connection-string keys must NOT be overwritten. Templates can carry
// values like POSTGRES_PASSWORD=changeme that the user may have already
// tailored; we should not silently flip them.
func TestEnrichEnvContentPreservesNonConnectionKeys(t *testing.T) {
	existing := "POSTGRES_PASSWORD=customsecret\nSECRET_KEY=abc\n"
	enriched := EnrichEnvContent(existing, map[string]string{
		"POSTGRES_PASSWORD": "app",
		"POSTGRES_USER":     "app",
	})
	if !strings.Contains(enriched, "POSTGRES_PASSWORD=customsecret") {
		t.Errorf("non-connection-string POSTGRES_PASSWORD was overwritten:\n%s", enriched)
	}
	if !strings.Contains(enriched, "POSTGRES_USER=app") {
		t.Errorf("expected POSTGRES_USER appended:\n%s", enriched)
	}
}

func TestEnrichEnvContentEmptyExisting(t *testing.T) {
	out := EnrichEnvContent("", map[string]string{"REDIS_URL": "redis://redis:6379/0"})
	if !strings.Contains(out, "REDIS_URL=redis://redis:6379/0") {
		t.Errorf("expected REDIS_URL set, got:\n%s", out)
	}
}

func TestReplaceEnvValuesPreservesCommentsAndUnrelatedKeys(t *testing.T) {
	got := ReplaceEnvValues("# API\nAPI_URL=http://localhost:8000\nOTHER=value\n", map[string]string{"API_URL": "http://service-2:8000"})
	want := "# API\nAPI_URL=http://service-2:8000\nOTHER=value\n"
	if got != want {
		t.Fatalf("replacement mismatch:\nwant %q\n got %q", want, got)
	}
}

func TestClearGeneratedConnectionPlaceholders(t *testing.T) {
	got := ClearGeneratedConnectionPlaceholders("DATABASE_URL=postgresql://user:pass@db:5432/app\nCUSTOM_URL=postgresql://real.example/db\n")
	want := "DATABASE_URL=\nCUSTOM_URL=postgresql://real.example/db\n"
	if got != want {
		t.Fatalf("placeholder clearing mismatch:\nwant %q\n got %q", want, got)
	}
}
