// Package infra infers backing-service dependencies (databases, caches,
// queues) from the environment variables an application references. The
// detected infra is materialised as additional compose services with
// healthchecks, default credentials, and matching connection strings written
// back into the application's .env.example.
//
// Inference is conservative: a service is only emitted when there is a
// reasonably high-signal hint (DATABASE_URL, POSTGRES_HOST, REDIS_URL,
// MONGO_URI, RABBITMQ_URL, ELASTIC_URL, etc.). Free-floating names such as
// "URL" or "HOST" are ignored.
package infra

import (
	"sort"
	"strings"

	"yoink/internal/envvar"
)

// Kind enumerates the infrastructure services Yoink knows how to provision.
type Kind string

const (
	KindPostgres      Kind = "postgres"
	KindMySQL         Kind = "mysql"
	KindRedis         Kind = "redis"
	KindMongo         Kind = "mongo"
	KindRabbitMQ      Kind = "rabbitmq"
	KindElasticsearch Kind = "elasticsearch"
	KindKafka         Kind = "kafka"
	KindMinIO         Kind = "minio"
)

// Service is one inferred infrastructure container.
type Service struct {
	Kind        Kind
	Name        string            // compose service name, e.g. "postgres"
	Image       string            // e.g. "postgres:16-alpine"
	Port        int               // container port (canonical)
	ExtraPorts  []int             // additional container ports (e.g. rabbitmq management)
	Env         map[string]string // environment fed into the infra container itself
	VolumeName  string            // named volume for persistent data ("" if stateless)
	VolumePath  string            // mount path inside the container
	Healthcheck Healthcheck
	Reason      string // which env hint triggered inference (for the UI)
	// Provider identifies the cloud/service provider when the infra is
	// external (e.g. "neon", "upstash", "atlas"). Empty when local.
	Provider string `json:"provider,omitempty"`
	// Mode is "local" (Yoink provisions a container) or "external" (the
	// project uses a cloud provider; Yoink should NOT provision locally).
	// "unknown" when insufficient evidence.
	Mode string `json:"mode,omitempty"`
}

// Healthcheck holds a compose-style healthcheck definition.
type Healthcheck struct {
	Test     []string // CMD or CMD-SHELL form, first element is "CMD"/"CMD-SHELL"
	Interval string
	Timeout  string
	Retries  int
}

// AppLink describes how an inferred infra service is wired into application
// services: the connection string env vars to inject and the depends-on
// service name.
type AppLink struct {
	ServiceName string            // infra service name (depends_on target)
	EnvVars     map[string]string // env vars to add/override on app services
}

// Inference is the full output of inference: the infra services to add plus
// the env-var injections each application service should receive.
type Inference struct {
	Services []Service
	// Links[appServiceID] = the list of infra services this app references and the env to inject.
	Links map[string][]AppLink
}

// Infer scans the env-var detection results and returns the infra services
// that should be added to the stack. Each application service is examined
// independently; the same infra type referenced by multiple apps is emitted
// once and shared. Deps are a second evidence source: e.g. "psycopg" →
// postgres, "redis" → redis, even when no DATABASE_URL env var exists.
//
// Provider detection: when a project uses a provider-specific SDK (e.g.
// @neondatabase/serverless, @upstash/redis), the infra is marked Mode=external
// and NOT provisioned locally. This prevents Yoink from spinning up a local
// postgres container when the app connects to Neon.
func Infer(results []envvar.Result) *Inference {
	out := &Inference{Links: map[string][]AppLink{}}
	bag := newBag()

	for _, r := range results {
		names := collectVarNames(r)
		content := strings.ToUpper(r.EnvContent)
		depSet := collectDepSet(r.Deps)

		// Provider detection: check for provider-specific deps/imports.
		// This runs BEFORE generic infra rules so external providers can
		// suppress unnecessary local provisioning.
		providerInfra := detectProviders(depSet, names)
		for kind, prov := range providerInfra {
			svc := bag.ensurePtr(kind)
			svc.Provider = prov.Provider
			svc.Mode = "external"
			svc.Reason = prov.Reason
			// External providers must never receive local connection strings. Keep
			// the link for graph/explain purposes, but preserve the repository's
			// own provider configuration unchanged.
			link := AppLink{ServiceName: svc.Name}
			out.Links[r.ServiceID] = append(out.Links[r.ServiceID], link)
		}

		// Env-var-based rules (strong signal). Skip if a provider already
		// claimed this Kind as external.
		for _, rule := range rules {
			if !rule.matches(names, content) {
				continue
			}
			svc := bag.ensurePtr(rule.kind)
			// Don't downgrade an external provider to local.
			if svc.Mode == "external" {
				link := AppLink{ServiceName: svc.Name}
				out.Links[r.ServiceID] = appendIfNew(out.Links[r.ServiceID], link)
				continue
			}
			if svc.Mode == "" {
				svc.Mode = "local"
			}
			link := AppLink{ServiceName: svc.Name, EnvVars: buildAppEnv(rule.kind, *svc)}
			out.Links[r.ServiceID] = appendIfNew(out.Links[r.ServiceID], link)
		}
		// Dep-based rules (medium signal).
		for _, rule := range depRules {
			if !rule.matchesDeps(depSet) {
				continue
			}
			svc := bag.ensurePtr(rule.kind)
			if svc.Mode == "external" {
				link := AppLink{ServiceName: svc.Name}
				appendIfNew(out.Links[r.ServiceID], link)
				continue
			}
			if svc.Mode == "" {
				svc.Mode = "local"
			}
			link := AppLink{ServiceName: svc.Name, EnvVars: buildAppEnv(rule.kind, *svc)}
			appendIfNew(out.Links[r.ServiceID], link)
		}
	}

	out.Services = bag.sorted()
	return out
}

// appendIfNew appends link to links only if no link with the same ServiceName
// already exists.
func appendIfNew(links []AppLink, link AppLink) []AppLink {
	for _, existing := range links {
		if existing.ServiceName == link.ServiceName {
			return links
		}
	}
	return append(links, link)
}

// ProviderEvidence captures a detected cloud provider for an infra kind.
type ProviderEvidence struct {
	Provider string // "neon", "upstash", "atlas", "confluent"
	Reason   string // human-readable evidence
}

// providerDepRules maps provider-specific dependency names to infra kinds.
// STRONG evidence: a project importing a provider SDK is almost certainly
// using that provider's cloud service, not a local container.
var providerDepRules = map[Kind][]providerDep{
	KindPostgres: {
		{deps: []string{"@neondatabase/serverless"}, provider: "neon"},
		{deps: []string{"@vercel/postgres"}, provider: "vercel"},
	},
	KindRedis: {
		{deps: []string{"@upstash/redis"}, provider: "upstash"},
	},
	KindMongo: {
		{deps: []string{"mongodb-atlas"}, provider: "atlas"},
	},
	KindKafka: {
		{deps: []string{"@confluentinc/kafka-javascript"}, provider: "confluent"},
	},
}

type providerDep struct {
	deps     []string
	provider string
}

// detectProviders checks for provider-specific dependencies and returns
// a map of Kind → ProviderEvidence. When a provider is detected, the infra
// is marked external — Yoink should NOT provision a local container.
func detectProviders(depSet map[string]bool, envNames map[string]bool) map[Kind]ProviderEvidence {
	out := map[Kind]ProviderEvidence{}
	for kind, rules := range providerDepRules {
		for _, rule := range rules {
			for _, dep := range rule.deps {
				if depSet[dep] {
					out[kind] = ProviderEvidence{
						Provider: rule.provider,
						Reason:   "dependency " + dep + " indicates " + rule.provider,
					}
					break
				}
			}
		}
	}
	// Also check env var names for provider-specific patterns.
	// e.g. NEON_DATABASE_URL, UPSTASH_REDIS_URL
	for name := range envNames {
		upper := strings.ToUpper(name)
		if strings.Contains(upper, "NEON") && out[KindPostgres] == (ProviderEvidence{}) {
			out[KindPostgres] = ProviderEvidence{Provider: "neon", Reason: "env var " + name}
		}
		if strings.Contains(upper, "UPSTASH") && out[KindRedis] == (ProviderEvidence{}) {
			out[KindRedis] = ProviderEvidence{Provider: "upstash", Reason: "env var " + name}
		}
	}
	return out
}

func collectDepSet(deps []string) map[string]bool {
	out := make(map[string]bool, len(deps))
	for _, d := range deps {
		out[strings.ToLower(d)] = true
	}
	return out
}

// connectionStringKeys are env vars whose value embeds a hostname pointing at
// a backing service. When infra inference provisions that backing service we
// want our connection string to win over any placeholder template value
// (e.g. the framework-default `DATABASE_URL=postgresql://user:pass@db:5432/app`
// in the common-vars seed should be replaced with the real `postgres:5432`
// hostname the compose file actually exposes).
var connectionStringKeys = map[string]bool{
	"DATABASE_URL":      true,
	"POSTGRES_URL":      true,
	"MYSQL_URL":         true,
	"REDIS_URL":         true,
	"CACHE_URL":         true,
	"MONGO_URI":         true,
	"MONGO_URL":         true,
	"MONGODB_URI":       true,
	"MONGODB_URL":       true,
	"RABBITMQ_URL":      true,
	"AMQP_URL":          true,
	"ELASTIC_URL":       true,
	"ELASTICSEARCH_URL": true,
}

// EnrichEnvContent merges injected env vars into the existing .env.example
// content. Existing assignments are preserved (the user/LLM-provided value
// wins) EXCEPT for connection-string keys, which we overwrite so the value
// points at the infra container we just provisioned instead of a placeholder
// hostname. Missing keys are appended in a clearly labelled section.
func EnrichEnvContent(existing string, env map[string]string) string {
	if len(env) == 0 {
		return existing
	}

	// Overwrite connection-string keys in-place.
	overwritten := map[string]bool{}
	if existing != "" {
		var b strings.Builder
		for i, line := range strings.Split(existing, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
				if key, _, ok := strings.Cut(trimmed, "="); ok {
					k := strings.TrimSpace(key)
					if connectionStringKeys[k] {
						if v, has := env[k]; has {
							line = k + "=" + v
							overwritten[k] = true
						}
					}
				}
			}
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(line)
		}
		existing = b.String()
	}

	have := existingKeys(existing)

	keys := make([]string, 0, len(env))
	for k := range env {
		if have[k] && !overwritten[k] {
			// Already present and not a connection-string we just overwrote.
			continue
		}
		if overwritten[k] {
			continue
		}
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return existing
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(strings.TrimRight(existing, "\n"))
	if existing != "" {
		b.WriteString("\n\n")
	}
	b.WriteString("# Backing services inferred by Yoink\n")
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(env[k])
		b.WriteByte('\n')
	}
	return b.String()
}

// ReplaceEnvValues updates explicit environment assignments without changing
// comments or unrelated keys. It is used for app-to-app internal URLs, where
// a generated Docker DNS value must win over a localhost build placeholder.
func ReplaceEnvValues(existing string, replacements map[string]string) string {
	if existing == "" || len(replacements) == 0 {
		return existing
	}
	var b strings.Builder
	for i, line := range strings.Split(existing, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			if key, _, ok := strings.Cut(trimmed, "="); ok {
				key = strings.TrimSpace(key)
				if value, found := replacements[key]; found {
					line = key + "=" + value
				}
			}
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
	}
	return b.String()
}

// ClearGeneratedConnectionPlaceholders removes only the framework defaults that
// Yoink itself seeded. Repository-provided provider URLs remain untouched.
func ClearGeneratedConnectionPlaceholders(existing string) string {
	placeholders := map[string]bool{
		"postgresql://user:pass@db:5432/app": true,
		"postgres://user:pass@db:5432/app":   true,
		"mysql://app:app@db:3306/app":        true,
		"redis://redis:6379/0":               true,
		"mongodb://mongo:27017/app":          true,
	}
	var b strings.Builder
	for i, line := range strings.Split(existing, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			if key, value, ok := strings.Cut(trimmed, "="); ok && placeholders[strings.TrimSpace(value)] {
				line = strings.TrimSpace(key) + "="
			}
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
	}
	return b.String()
}

func existingKeys(content string) map[string]bool {
	out := map[string]bool{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, _, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(key)] = true
	}
	return out
}

// collectVarNames returns the upper-cased set of env-var names attached to
// this result, drawn from both the detector hits and any committed env-file
// content.
func collectVarNames(r envvar.Result) map[string]bool {
	out := map[string]bool{}
	for _, v := range r.Vars {
		out[strings.ToUpper(v.Name)] = true
	}
	for _, line := range strings.Split(r.EnvContent, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, _, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.ToUpper(strings.TrimSpace(key))] = true
	}
	return out
}

// rule maps env-var hints to a Kind and a connection-string template.
type rule struct {
	kind      Kind
	hints     []string // exact name matches
	prefixes  []string // any name starting with one of these matches
	substring []string // content substrings to look for as additional signal
}

func (r rule) matches(names map[string]bool, content string) bool {
	for _, n := range r.hints {
		if names[n] {
			return true
		}
	}
	for _, p := range r.prefixes {
		for n := range names {
			if strings.HasPrefix(n, p) {
				return true
			}
		}
	}
	for _, s := range r.substring {
		if strings.Contains(content, s) {
			return true
		}
	}
	return false
}

func buildAppEnv(kind Kind, svc Service) map[string]string {
	switch kind {
	case KindPostgres:
		return map[string]string{
			"DATABASE_URL":      "postgresql://app:app@" + svc.Name + ":5432/app",
			"POSTGRES_HOST":     svc.Name,
			"POSTGRES_PORT":     "5432",
			"POSTGRES_DB":       "app",
			"POSTGRES_USER":     "app",
			"POSTGRES_PASSWORD": "app",
		}
	case KindMySQL:
		return map[string]string{
			"DATABASE_URL":   "mysql://app:app@" + svc.Name + ":3306/app",
			"MYSQL_HOST":     svc.Name,
			"MYSQL_PORT":     "3306",
			"MYSQL_DATABASE": "app",
			"MYSQL_USER":     "app",
			"MYSQL_PASSWORD": "app",
		}
	case KindRedis:
		return map[string]string{
			"REDIS_URL":  "redis://" + svc.Name + ":6379/0",
			"REDIS_HOST": svc.Name,
			"REDIS_PORT": "6379",
		}
	case KindMongo:
		return map[string]string{
			"MONGO_URI":   "mongodb://" + svc.Name + ":27017/app",
			"MONGODB_URL": "mongodb://" + svc.Name + ":27017/app",
			"MONGO_HOST":  svc.Name,
			"MONGO_PORT":  "27017",
		}
	case KindRabbitMQ:
		return map[string]string{
			"RABBITMQ_URL":  "amqp://guest:guest@" + svc.Name + ":5672/",
			"RABBITMQ_HOST": svc.Name,
			"RABBITMQ_PORT": "5672",
		}
	case KindElasticsearch:
		return map[string]string{
			"ELASTICSEARCH_URL": "http://" + svc.Name + ":9200",
			"ELASTIC_URL":       "http://" + svc.Name + ":9200",
			"ELASTIC_HOST":      svc.Name,
			"ELASTIC_PORT":      "9200",
		}
	case KindKafka:
		return map[string]string{
			"KAFKA_URL":               svc.Name + ":9092",
			"KAFKA_BROKERS":           svc.Name + ":9092",
			"KAFKA_BOOTSTRAP_SERVERS": svc.Name + ":9092",
		}
	case KindMinIO:
		return map[string]string{
			"MINIO_ENDPOINT":   "http://" + svc.Name + ":9000",
			"MINIO_URL":        "http://" + svc.Name + ":9000",
			"MINIO_HOST":       svc.Name,
			"MINIO_PORT":       "9000",
			"MINIO_ACCESS_KEY": "minio",
			"MINIO_SECRET_KEY": "minio123",
		}
	}
	return nil
}

var rules = []rule{
	{
		kind:      KindPostgres,
		hints:     []string{"DATABASE_URL", "POSTGRES_URL", "POSTGRES_HOST", "POSTGRES_DB", "POSTGRES_USER", "POSTGRES_PASSWORD", "PG_URL", "PGHOST", "PGDATABASE"},
		prefixes:  []string{"POSTGRES_"},
		substring: []string{"POSTGRES://", "POSTGRESQL://"},
	},
	{
		kind:      KindMySQL,
		hints:     []string{"MYSQL_URL", "MYSQL_HOST", "MYSQL_DATABASE", "MYSQL_USER", "MYSQL_PASSWORD", "MYSQL_ROOT_PASSWORD"},
		prefixes:  []string{"MYSQL_"},
		substring: []string{"MYSQL://"},
	},
	{
		kind:      KindRedis,
		hints:     []string{"REDIS_URL", "REDIS_HOST", "REDIS_PORT", "CACHE_URL"},
		prefixes:  []string{"REDIS_"},
		substring: []string{"REDIS://"},
	},
	{
		kind:      KindMongo,
		hints:     []string{"MONGO_URI", "MONGODB_URL", "MONGODB_URI", "MONGO_URL", "MONGO_HOST"},
		prefixes:  []string{"MONGO_", "MONGODB_"},
		substring: []string{"MONGODB://"},
	},
	{
		kind:      KindRabbitMQ,
		hints:     []string{"RABBITMQ_URL", "RABBITMQ_HOST", "RABBITMQ_PORT", "AMQP_URL"},
		prefixes:  []string{"RABBITMQ_"},
		substring: []string{"AMQP://"},
	},
	{
		kind:     KindElasticsearch,
		hints:    []string{"ELASTIC_URL", "ELASTICSEARCH_URL", "ELASTIC_HOST", "ELASTICSEARCH_HOST"},
		prefixes: []string{"ELASTIC_", "ELASTICSEARCH_"},
	},
	{
		kind:     KindKafka,
		hints:    []string{"KAFKA_URL", "KAFKA_BROKERS", "KAFKA_BOOTSTRAP_SERVERS", "KAFKA_HOST"},
		prefixes: []string{"KAFKA_"},
	},
	{
		kind:     KindMinIO,
		hints:    []string{"MINIO_ENDPOINT", "MINIO_URL", "MINIO_HOST", "S3_ENDPOINT"},
		prefixes: []string{"MINIO_"},
	},
}

// depRule maps runtime dependency names to an infra Kind. This is a second
// evidence source: a project that imports psycopg/pg/redis/mongoose doesn't
// need a DATABASE_URL env var for Yoink to infer the backing service.
type depRule struct {
	kind Kind
	deps []string // exact lowercased dep name matches
}

func (r depRule) matchesDeps(depSet map[string]bool) bool {
	for _, d := range r.deps {
		if depSet[d] {
			return true
		}
	}
	return false
}

// depRules maps package dependency names to infra kinds. Covers both Python
// (psycopg, redis, pymongo, …) and JS (pg, ioredis, mongoose, …) ecosystems.
var depRules = []depRule{
	{KindPostgres, []string{"psycopg", "psycopg2", "psycopg2-binary", "asyncpg", "pg", "postgres", "sqlalchemy", "sqlmodel", "psycopg[binary]", "psycopg2-binary", "psycopg-c", "psycopg-pool"}},
	{KindMySQL, []string{"mysqlclient", "pymysql", "mysql-connector-python", "aiomysql", "mysql2", "mysqlconnector"}},
	{KindRedis, []string{"redis", "aioredis", "ioredis", "redis-py", "redis-om"}},
	{KindMongo, []string{"pymongo", "motor", "mongoose", "mongodb"}},
	{KindRabbitMQ, []string{"pika", "aio-pika", "amqplib", "rabbitpy"}},
	{KindElasticsearch, []string{"elasticsearch", "opensearch-py", "@elastic/elasticsearch"}},
	{KindKafka, []string{"kafka-python", "aiokafka", "kafkajs", "confluent-kafka"}},
	{KindMinIO, []string{"minio", "minio-go"}},
}

// bag accumulates inferred infra services so we emit at most one of each kind.
type bag struct {
	services map[Kind]*Service
	order    []Kind
}

func newBag() *bag { return &bag{services: map[Kind]*Service{}} }

// ensurePtr returns a pointer to the service in the bag, so callers can
// modify fields (e.g. Provider, Mode) that persist in the bag's state.
func (b *bag) ensurePtr(k Kind) *Service {
	if s, ok := b.services[k]; ok {
		return s
	}
	s := defaultService(k)
	b.services[k] = &s
	b.order = append(b.order, k)
	return b.services[k]
}

func (b *bag) sorted() []Service {
	out := make([]Service, 0, len(b.order))
	seen := map[Kind]bool{}
	for _, k := range b.order {
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, *b.services[k])
	}
	return out
}

func defaultService(k Kind) Service {
	switch k {
	case KindPostgres:
		return Service{
			Kind:  KindPostgres,
			Name:  "postgres",
			Image: "postgres:16-alpine",
			Port:  5432,
			Env: map[string]string{
				"POSTGRES_DB":       "app",
				"POSTGRES_USER":     "app",
				"POSTGRES_PASSWORD": "app",
			},
			VolumeName: "yoink-postgres-data",
			VolumePath: "/var/lib/postgresql/data",
			Healthcheck: Healthcheck{
				Test:     []string{"CMD-SHELL", "pg_isready -U app -d app"},
				Interval: "5s", Timeout: "3s", Retries: 10,
			},
			Reason: "DATABASE_URL / POSTGRES_*",
		}
	case KindMySQL:
		return Service{
			Kind:  KindMySQL,
			Name:  "mysql",
			Image: "mysql:8",
			Port:  3306,
			Env: map[string]string{
				"MYSQL_DATABASE":      "app",
				"MYSQL_USER":          "app",
				"MYSQL_PASSWORD":      "app",
				"MYSQL_ROOT_PASSWORD": "rootpw",
			},
			VolumeName: "yoink-mysql-data",
			VolumePath: "/var/lib/mysql",
			Healthcheck: Healthcheck{
				Test:     []string{"CMD-SHELL", "mysqladmin ping -h localhost --silent"},
				Interval: "5s", Timeout: "3s", Retries: 15,
			},
			Reason: "MYSQL_*",
		}
	case KindRedis:
		return Service{
			Kind:  KindRedis,
			Name:  "redis",
			Image: "redis:7-alpine",
			Port:  6379,
			Healthcheck: Healthcheck{
				Test:     []string{"CMD", "redis-cli", "ping"},
				Interval: "5s", Timeout: "3s", Retries: 10,
			},
			Reason: "REDIS_URL / REDIS_HOST",
		}
	case KindMongo:
		return Service{
			Kind:       KindMongo,
			Name:       "mongo",
			Image:      "mongo:7",
			Port:       27017,
			VolumeName: "yoink-mongo-data",
			VolumePath: "/data/db",
			Healthcheck: Healthcheck{
				Test:     []string{"CMD-SHELL", "mongosh --quiet --eval \"db.runCommand({ping:1}).ok\" | grep -q 1"},
				Interval: "5s", Timeout: "5s", Retries: 12,
			},
			Reason: "MONGO_URI / MONGODB_URL",
		}
	case KindRabbitMQ:
		return Service{
			Kind:       KindRabbitMQ,
			Name:       "rabbitmq",
			Image:      "rabbitmq:3-management",
			Port:       5672,
			ExtraPorts: []int{15672},
			Env: map[string]string{
				"RABBITMQ_DEFAULT_USER": "guest",
				"RABBITMQ_DEFAULT_PASS": "guest",
			},
			Healthcheck: Healthcheck{
				Test:     []string{"CMD", "rabbitmq-diagnostics", "-q", "ping"},
				Interval: "10s", Timeout: "5s", Retries: 10,
			},
			Reason: "RABBITMQ_URL / AMQP_URL",
		}
	case KindElasticsearch:
		return Service{
			Kind:  KindElasticsearch,
			Name:  "elasticsearch",
			Image: "docker.elastic.co/elasticsearch/elasticsearch:8.13.4",
			Port:  9200,
			Env: map[string]string{
				"discovery.type":         "single-node",
				"xpack.security.enabled": "false",
				"ES_JAVA_OPTS":           "-Xms512m -Xmx512m",
			},
			VolumeName: "yoink-elastic-data",
			VolumePath: "/usr/share/elasticsearch/data",
			Healthcheck: Healthcheck{
				Test:     []string{"CMD-SHELL", "curl -fs http://localhost:9200/_cluster/health || exit 1"},
				Interval: "10s", Timeout: "5s", Retries: 12,
			},
			Reason: "ELASTIC_URL / ELASTICSEARCH_URL",
		}
	case KindKafka:
		return Service{
			Kind:  KindKafka,
			Name:  "kafka",
			Image: "confluentinc/cp-kafka:7.6.0",
			Port:  9092,
			Env: map[string]string{
				"KAFKA_ADVERTISED_LISTENERS":             "PLAINTEXT://kafka:9092",
				"KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR": "1",
				"KAFKA_AUTO_CREATE_TOPICS_ENABLE":        "true",
			},
			Reason: "KAFKA_URL / KAFKA_* / kafka-python",
		}
	case KindMinIO:
		return Service{
			Kind:  KindMinIO,
			Name:  "minio",
			Image: "minio/minio:latest",
			Port:  9000,
			Env: map[string]string{
				"MINIO_ROOT_USER":     "minio",
				"MINIO_ROOT_PASSWORD": "minio123",
			},
			VolumeName: "yoink-minio-data",
			VolumePath: "/data",
			Healthcheck: Healthcheck{
				Test:     []string{"CMD-SHELL", "curl -fs http://localhost:9000/minio/health/live || exit 1"},
				Interval: "10s", Timeout: "5s", Retries: 12,
			},
			Reason: "MINIO_* / minio client",
		}
	}
	return Service{Kind: k, Name: string(k)}
}
