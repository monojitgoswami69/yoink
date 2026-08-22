package infra

import (
	"testing"

	"yoink/internal/envvar"
)

func TestInferFromPythonDeps(t *testing.T) {
	results := []envvar.Result{
		{ServiceID: "s1", Technology: "fastapi", Deps: []string{"fastapi", "psycopg", "uvicorn"}},
	}
	inf := Infer(results)
	if len(inf.Services) != 1 {
		t.Fatalf("expected 1 infra service from psycopg dep, got %d", len(inf.Services))
	}
	if inf.Services[0].Kind != KindPostgres {
		t.Errorf("expected postgres, got %s", inf.Services[0].Kind)
	}
}

func TestInferFromJSDeps(t *testing.T) {
	results := []envvar.Result{
		{ServiceID: "s1", Technology: "express", Deps: []string{"express", "ioredis", "pg"}},
	}
	inf := Infer(results)
	if len(inf.Services) != 2 {
		t.Fatalf("expected 2 infra services (redis+postgres) from deps, got %d: %+v", len(inf.Services), inf.Services)
	}
	kinds := map[Kind]bool{}
	for _, s := range inf.Services {
		kinds[s.Kind] = true
	}
	if !kinds[KindRedis] || !kinds[KindPostgres] {
		t.Errorf("expected redis+postgres, got %v", kinds)
	}
}

func TestInferFromMongoDep(t *testing.T) {
	results := []envvar.Result{
		{ServiceID: "s1", Technology: "express", Deps: []string{"mongoose"}},
	}
	inf := Infer(results)
	if len(inf.Services) != 1 || inf.Services[0].Kind != KindMongo {
		t.Errorf("expected mongo from mongoose dep, got %+v", inf.Services)
	}
}

func TestInferDoesNotDuplicateFromEnvAndDeps(t *testing.T) {
	// Both env var (DATABASE_URL) and dep (psycopg) point at postgres.
	// Should emit postgres once, not twice.
	results := []envvar.Result{
		{
			ServiceID:  "s1",
			Technology: "fastapi",
			Vars:       []envvar.EnvVar{{Name: "DATABASE_URL"}},
			Deps:       []string{"psycopg"},
		},
	}
	inf := Infer(results)
	count := 0
	for _, s := range inf.Services {
		if s.Kind == KindPostgres {
			count++
		}
	}
	if count != 1 {
		t.Errorf("postgres should appear once (dedup env+dep), got %d", count)
	}
}

func TestInferFromKafkaDep(t *testing.T) {
	results := []envvar.Result{
		{ServiceID: "s1", Technology: "node", Deps: []string{"kafkajs"}},
	}
	inf := Infer(results)
	if len(inf.Services) != 1 || inf.Services[0].Kind != KindKafka {
		t.Errorf("expected kafka from kafkajs dep, got %+v", inf.Services)
	}
}

func TestInferFromMinIODep(t *testing.T) {
	results := []envvar.Result{
		{ServiceID: "s1", Technology: "node", Deps: []string{"minio"}},
	}
	inf := Infer(results)
	if len(inf.Services) != 1 || inf.Services[0].Kind != KindMinIO {
		t.Errorf("expected minio from minio dep, got %+v", inf.Services)
	}
}
