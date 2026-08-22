package llm

import "testing"

const goodCompose = `services:
  service-1:
    build:
      context: ..
      dockerfile: yoink-outputs/Dockerfile.service-1
    ports:
      - "8000:8000"

  postgres:
    image: postgres:16-alpine
    ports:
      - "5432:5432"
`

func TestAssertComposeLayoutAcceptsGenerated(t *testing.T) {
	if err := AssertComposeLayout(goodCompose); err != nil {
		t.Fatalf("expected canonical compose to pass, got %v", err)
	}
}

func TestAssertComposeLayoutRejectsDotContext(t *testing.T) {
	bad := `services:
  service-1:
    build:
      context: .
      dockerfile: Dockerfile.service-1
`
	if err := AssertComposeLayout(bad); err == nil {
		t.Fatal("expected rejection when build.context is rewritten to '.'")
	}
}

func TestAssertComposeLayoutRejectsBareDockerfile(t *testing.T) {
	bad := `services:
  service-1:
    build:
      context: ..
      dockerfile: Dockerfile.service-1
`
	if err := AssertComposeLayout(bad); err == nil {
		t.Fatal("expected rejection when Dockerfile path drops the yoink-outputs/ prefix")
	}
}

func TestAssertComposeLayoutIgnoresImageOnlyServices(t *testing.T) {
	ok := `services:
  service-1:
    build:
      context: ..
      dockerfile: yoink-outputs/Dockerfile.service-1
  redis:
    image: redis:7-alpine
`
	if err := AssertComposeLayout(ok); err != nil {
		t.Fatalf("infra services without build: should not be checked: %v", err)
	}
}

func TestAssertComposeLayoutRejectsEmpty(t *testing.T) {
	if err := AssertComposeLayout(""); err == nil {
		t.Fatal("expected rejection for empty compose")
	}
}
