package generator

import (
	"strings"
	"testing"

	"yoink/internal/detector"
)

// TestWriteBuildEnvDoesNotBakeSecrets proves that secret-named env vars
// (JWT_SECRET, API_KEY, PASSWORD, TOKEN, etc.) are NEVER emitted with their
// real value as ENV directives in the Dockerfile. They always get the
// generic "yoink-build-placeholder" so real secrets never become image-layer
// data. Non-secret configuration (PORT, DATABASE_URL, NODE_ENV) uses real
// values.
func TestWriteBuildEnvDoesNotBakeSecrets(t *testing.T) {
	svc := detector.Service{
		BuildEnv: map[string]string{
			"JWT_SECRET":   "real-secret-value-12345",
			"API_KEY":      "sk-1234567890abcdef",
			"DATABASE_URL": "postgresql://app:app@postgres:5432/app",
			"PORT":         "3000",
			"NODE_ENV":     "production",
			"PASSWORD":     "super-secret-password",
		},
	}
	var b strings.Builder
	writeBuildEnv(&b, svc)
	output := b.String()

	// Secret vars must NOT leak their real values.
	if strings.Contains(output, "real-secret-value-12345") {
		t.Error("JWT_SECRET real value leaked into Dockerfile ENV")
	}
	if strings.Contains(output, "sk-1234567890abcdef") {
		t.Error("API_KEY real value leaked into Dockerfile ENV")
	}
	if strings.Contains(output, "super-secret-password") {
		t.Error("PASSWORD real value leaked into Dockerfile ENV")
	}

	// Secret vars should use the placeholder.
	if !strings.Contains(output, "ENV JWT_SECRET=yoink-build-placeholder") {
		t.Error("JWT_SECRET should use placeholder, got:\n" + output)
	}
	if !strings.Contains(output, "ENV API_KEY=yoink-build-placeholder") {
		t.Error("API_KEY should use placeholder, got:\n" + output)
	}
	if !strings.Contains(output, "ENV PASSWORD=yoink-build-placeholder") {
		t.Error("PASSWORD should use placeholder, got:\n" + output)
	}

	// Non-secret config should use real values.
	if !strings.Contains(output, "ENV DATABASE_URL=postgresql://app:app@postgres:5432/app") {
		t.Error("DATABASE_URL should use real inferred value, got:\n" + output)
	}
	if !strings.Contains(output, "ENV PORT=3000") {
		t.Error("PORT should use real value")
	}
	if !strings.Contains(output, "ENV NODE_ENV=production") {
		t.Error("NODE_ENV should use real value")
	}
}

// TestIsSecretEnvVar verifies the secret classification heuristic.
func TestIsSecretEnvVar(t *testing.T) {
	secrets := []string{
		"JWT_SECRET", "API_KEY", "DATABASE_PASSWORD", "ACCESS_TOKEN",
		"REFRESH_TOKEN", "CREDENTIAL", "PRIVATE_KEY", "SECRET_KEY",
		"OPENAI_API_KEY", "GITHUB_TOKEN", "AWS_SECRET_ACCESS_KEY",
	}
	for _, s := range secrets {
		if !isSecretEnvVar(s) {
			t.Errorf("isSecretEnvVar(%q) = false, want true", s)
		}
	}
	nonSecrets := []string{
		"PORT", "NODE_ENV", "DATABASE_URL", "HOST", "NEXT_PUBLIC_API_URL",
		"VITE_API_URL", "DEBUG", "LOG_LEVEL", "REDIS_URL",
	}
	for _, s := range nonSecrets {
		if isSecretEnvVar(s) {
			t.Errorf("isSecretEnvVar(%q) = true, want false", s)
		}
	}
}

func TestURLBuildEnvUsesParseablePlaceholder(t *testing.T) {
	svc := detector.Service{BuildEnv: map[string]string{"UPSTASH_REDIS_REST_URL": ""}}
	var b strings.Builder
	writeBuildEnv(&b, svc)
	if !strings.Contains(b.String(), "ENV UPSTASH_REDIS_REST_URL=http://yoink-build-placeholder.invalid") {
		t.Fatalf("URL env should use a parseable placeholder: %s", b.String())
	}
}

func TestBuildEnvQuotesDockerValuesWithSpaces(t *testing.T) {
	svc := detector.Service{BuildEnv: map[string]string{"SEED_ADMIN_NAME": "Siksha Saathi Administrator"}}
	var b strings.Builder
	writeBuildEnv(&b, svc)
	if !strings.Contains(b.String(), `ENV SEED_ADMIN_NAME="Siksha Saathi Administrator"`) {
		t.Fatalf("space-containing ENV value was not quoted: %s", b.String())
	}
}
