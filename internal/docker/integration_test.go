package docker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// This is opt-in because it pulls images and requires a running daemon.
func TestComposeIntegrationViteFixture(t *testing.T) {
	if os.Getenv("YOINK_DOCKER_INTEGRATION") != "1" {
		t.Skip("set YOINK_DOCKER_INTEGRATION=1 to run Docker integration tests")
	}
	if !DaemonRunning(context.Background()) {
		t.Skip("Docker daemon unavailable")
	}
	root := t.TempDir()
	composePath := filepath.Join(root, "docker-compose.yml")
	compose := `services:
  app:
    image: nginx:alpine
    ports:
      - "18080:80"
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://localhost/ || exit 1"]
      interval: 1s
      timeout: 1s
      retries: 10
`
	if err := os.WriteFile(composePath, []byte(compose), 0644); err != nil {
		t.Fatal(err)
	}
	cm := New(composePath, root, "yoink-integration")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	t.Cleanup(func() { _, _ = cm.Down(context.Background(), true) })
	if out, err := cm.Up(ctx); err != nil {
		t.Fatalf("compose up: %v\n%s", err, out)
	}
	if out, err := cm.Ps(ctx); err != nil || len(out) != 1 || out[0].State != "running" {
		t.Fatalf("compose ps: err=%v containers=%+v", err, out)
	}
}
