package infra

import (
	"testing"

	"yoink/internal/envvar"
)

func TestInferNeonFromDep(t *testing.T) {
	results := []envvar.Result{
		{ServiceID: "s1", Technology: "next", Deps: []string{"next", "@neondatabase/serverless"}},
	}
	inf := Infer(results)
	if len(inf.Services) != 1 {
		t.Fatalf("expected 1 infra service, got %d", len(inf.Services))
	}
	svc := inf.Services[0]
	if svc.Kind != KindPostgres {
		t.Errorf("kind: want postgres, got %s", svc.Kind)
	}
	if svc.Provider != "neon" {
		t.Errorf("provider: want neon, got %s", svc.Provider)
	}
	if svc.Mode != "external" {
		t.Errorf("mode: want external, got %s", svc.Mode)
	}
}

func TestInferUpstashFromDep(t *testing.T) {
	results := []envvar.Result{
		{ServiceID: "s1", Technology: "next", Deps: []string{"@upstash/redis"}},
	}
	inf := Infer(results)
	if len(inf.Services) != 1 {
		t.Fatalf("expected 1 infra, got %d", len(inf.Services))
	}
	if inf.Services[0].Provider != "upstash" {
		t.Errorf("provider: want upstash, got %s", inf.Services[0].Provider)
	}
	if inf.Services[0].Mode != "external" {
		t.Errorf("mode: want external, got %s", inf.Services[0].Mode)
	}
}

func TestExternalProviderNotProvisionedLocally(t *testing.T) {
	results := []envvar.Result{
		{
			ServiceID: "s1", Technology: "next",
			Deps: []string{"@neondatabase/serverless"},
			Vars: []envvar.EnvVar{{Name: "DATABASE_URL"}},
		},
	}
	inf := Infer(results)
	if len(inf.Services) != 1 {
		t.Fatalf("got %d services", len(inf.Services))
	}
	if inf.Services[0].Mode != "external" {
		t.Error("Neon dep should force external mode even with DATABASE_URL")
	}
	if links := inf.Links["s1"]; len(links) != 1 || len(links[0].EnvVars) != 0 {
		t.Fatalf("external provider must not inject local env vars: %+v", links)
	}
}

func TestProviderDedupWithGenericDeps(t *testing.T) {
	results := []envvar.Result{
		{
			ServiceID: "s1", Technology: "next",
			Deps: []string{"@neondatabase/serverless", "pg"},
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
		t.Errorf("postgres should appear once (provider+dep dedup), got %d", count)
	}
}

func TestGenericPostgresDepIsLocal(t *testing.T) {
	results := []envvar.Result{
		{ServiceID: "s1", Technology: "fastapi", Deps: []string{"psycopg"}},
	}
	inf := Infer(results)
	if len(inf.Services) != 1 {
		t.Fatalf("got %d", len(inf.Services))
	}
	if inf.Services[0].Mode != "local" {
		t.Errorf("psycopg without provider should be local, got %s", inf.Services[0].Mode)
	}
}

func TestNoProviderFromUnrelatedDep(t *testing.T) {
	results := []envvar.Result{
		{ServiceID: "s1", Technology: "express", Deps: []string{"express", "lodash"}},
	}
	inf := Infer(results)
	if len(inf.Services) != 0 {
		t.Errorf("no infra expected from express+lodash, got %d", len(inf.Services))
	}
}
