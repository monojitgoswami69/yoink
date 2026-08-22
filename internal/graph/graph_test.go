package graph

import (
	"testing"

	"yoink/internal/detector"
	"yoink/internal/infra"
)

func app(id, fw, lang, typ string, port int, be map[string]string) detector.Service {
	return detector.Service{ID: id, Framework: fw, Language: lang, Type: typ, Port: port, BuildEnv: be}
}

func infraLocal(name string, kind infra.Kind, port int) infra.Service {
	return infra.Service{Name: name, Kind: kind, Mode: "local", Port: port, Reason: "env DATABASE_URL"}
}
func infraExternal(name string, kind infra.Kind, provider string) infra.Service {
	return infra.Service{Name: name, Kind: kind, Mode: "external", Provider: provider, Reason: "env var " + provider}
}

func TestGraphBackendToPostgresLocal(t *testing.T) {
	g := Build(
		[]detector.Service{app("service-1", "fastapi", "python", "backend", 8000, nil)},
		[]infra.Service{infraLocal("postgres", infra.KindPostgres, 5432)},
		map[string][]infra.AppLink{"service-1": {{ServiceName: "postgres", EnvVars: map[string]string{"DATABASE_URL": "postgres://postgres:5432/db"}}}},
		map[string]int{"service-1": 8001},
	)
	if n := g.Find("postgres"); n == nil || n.Kind != NodeInfra {
		t.Fatalf("postgres should be a local infra node: %+v", n)
	}
	if n := g.Find("service-1"); n == nil || n.Kind != NodeApp || n.HostPort != 8001 {
		t.Fatalf("service-1 app node wrong: %+v", n)
	}
	edges := g.EdgesFrom("service-1")
	if len(edges) != 1 || edges[0].To != "postgres" || edges[0].InternalURL != "http://postgres:5432" {
		t.Fatalf("expected one depends edge -> postgres, got %+v", edges)
	}
}

func TestGraphBackendToRedisLocal(t *testing.T) {
	g := Build(
		[]detector.Service{app("service-1", "express", "javascript", "backend", 3000, nil)},
		[]infra.Service{infraLocal("redis", infra.KindRedis, 6379)},
		map[string][]infra.AppLink{"service-1": {{ServiceName: "redis", EnvVars: map[string]string{"REDIS_URL": "redis://redis:6379"}}}},
		map[string]int{"service-1": 3001},
	)
	if g.IsExternal("redis") {
		t.Fatal("redis should be local, not external")
	}
	if e := g.EdgesFrom("service-1"); len(e) != 1 || e[0].To != "redis" {
		t.Fatalf("missing redis dependency: %+v", e)
	}
}

func TestGraphExternalNeon(t *testing.T) {
	g := Build(
		[]detector.Service{app("service-1", "next", "typescript", "frontend", 3000, nil)},
		[]infra.Service{infraExternal("neon", infra.KindPostgres, "neon")},
		map[string][]infra.AppLink{"service-1": {{ServiceName: "neon", EnvVars: map[string]string{"DATABASE_URL": "postgres://neon"}}}},
		map[string]int{"service-1": 3000},
	)
	n := g.Find("neon")
	if n == nil || n.Kind != NodeExternal || n.Provider != "neon" {
		t.Fatalf("neon must be an external node: %+v", n)
	}
	if !g.IsExternal("neon") {
		t.Error("IsExternal(neon) should be true")
	}
	// External infra is never started locally.
	for _, id := range g.StartOrder() {
		if id == "neon" {
			t.Error("external infra must not appear in StartOrder")
		}
	}
}

func TestGraphExternalUpstash(t *testing.T) {
	g := Build(
		[]detector.Service{app("service-1", "next", "typescript", "frontend", 3000, nil)},
		[]infra.Service{infraExternal("upstash", infra.KindRedis, "upstash")},
		map[string][]infra.AppLink{"service-1": {{ServiceName: "upstash", EnvVars: map[string]string{"UPSTASH_REDIS_REST_URL": "https://upstash"}}}},
		map[string]int{"service-1": 3000},
	)
	if n := g.Find("upstash"); n == nil || n.Kind != NodeExternal || n.Provider != "upstash" {
		t.Fatalf("upstash must be external: %+v", n)
	}
}

func TestGraphMultipleFrontends(t *testing.T) {
	svcs := []detector.Service{
		app("service-1", "vite", "typescript", "frontend", 5173, nil),
		app("service-2", "vite", "typescript", "frontend", 5174, nil),
		app("service-3", "fastapi", "python", "backend", 8000, nil),
	}
	g := Build(svcs, nil, nil, map[string]int{"service-1": 1, "service-2": 2, "service-3": 3})
	apps := g.PublicAppNodes()
	if len(apps) != 3 {
		t.Fatalf("expected 3 public app nodes, got %d", len(apps))
	}
}

func TestGraphAmbiguousPortBindingUnresolved(t *testing.T) {
	// Two backends on the same port 8000 -> a frontend env var referencing
	// :8000 is ambiguous and must NOT produce an app->app edge.
	svcs := []detector.Service{
		app("frontend", "next", "typescript", "frontend", 3000, map[string]string{"API_URL": "http://backend:8000"}),
		app("backend-a", "fastapi", "python", "backend", 8000, nil),
		app("backend-b", "flask", "python", "backend", 8000, nil),
	}
	g := Build(svcs, nil, nil, map[string]int{"frontend": 3000, "backend-a": 8001, "backend-b": 8002})
	for _, e := range g.Edges {
		if e.Kind == EdgeEnvBinding {
			t.Fatalf("ambiguous port binding must not produce an env-binding edge: %+v", e)
		}
	}
}

func TestGraphAppAppEnvBindingUnambiguous(t *testing.T) {
	// Frontend's build env API_URL references port 8000; exactly one backend
	// exposes 8000 -> an env-binding edge frontend -> backend is added.
	svcs := []detector.Service{
		app("frontend", "next", "typescript", "frontend", 3000, map[string]string{"API_URL": "http://backend:8000"}),
		app("backend", "fastapi", "python", "backend", 8000, nil),
	}
	g := Build(svcs, nil, nil, map[string]int{"frontend": 3000, "backend": 8001})
	var binding *Edge
	for i, e := range g.Edges {
		if e.Kind == EdgeEnvBinding && e.From == "frontend" && e.To == "backend" {
			binding = &g.Edges[i]
		}
	}
	if binding == nil {
		t.Fatalf("expected env-binding edge frontend->backend, got %+v", g.Edges)
	}
	if binding.InternalURL != "http://backend:8000" || binding.ExternalURL != "http://localhost:8001" {
		t.Errorf("binding urls wrong: int=%s ext=%s", binding.InternalURL, binding.ExternalURL)
	}
}

func TestGraphStartOrderInfraFirst(t *testing.T) {
	g := Build(
		[]detector.Service{
			app("service-1", "next", "typescript", "frontend", 3000, nil),
			app("service-2", "fastapi", "python", "backend", 8000, nil),
		},
		[]infra.Service{infraLocal("postgres", infra.KindPostgres, 5432), infraLocal("redis", infra.KindRedis, 6379)},
		map[string][]infra.AppLink{
			"service-2": {{ServiceName: "postgres", EnvVars: map[string]string{"DATABASE_URL": "x"}}},
			"service-1": {{ServiceName: "redis", EnvVars: map[string]string{"REDIS_URL": "x"}}},
		},
		map[string]int{"service-1": 3000, "service-2": 8000},
	)
	order := g.StartOrder()
	if len(order) == 0 {
		t.Fatal("empty start order")
	}
	// All local infra must precede all app nodes.
	firstApp := -1
	for i, id := range order {
		if n := g.Find(id); n != nil && n.Kind == NodeApp {
			firstApp = i
			break
		}
	}
	if firstApp < 0 {
		t.Fatal("no app in start order")
	}
	for i := 0; i < firstApp; i++ {
		if n := g.Find(order[i]); n == nil || n.Kind != NodeInfra {
			t.Errorf("infra must come before apps; got %s at %d", order[i], i)
		}
	}
}

func TestGraphPlaceholderEnvIgnored(t *testing.T) {
	svcs := []detector.Service{
		app("frontend", "next", "typescript", "frontend", 3000, map[string]string{"API_URL": "yoink-build-placeholder"}),
		app("backend", "fastapi", "python", "backend", 8000, nil),
	}
	g := Build(svcs, nil, nil, map[string]int{"frontend": 3000, "backend": 8001})
	for _, e := range g.Edges {
		if e.Kind == EdgeEnvBinding {
			t.Fatalf("placeholder env must not bind: %+v", e)
		}
	}
}
