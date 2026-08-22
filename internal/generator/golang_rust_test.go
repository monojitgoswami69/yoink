package generator

import (
	"strings"
	"testing"

	"yoink/internal/detector"
)

func TestRenderGoDockerfile(t *testing.T) {
	svc := detector.Service{
		ID: "s1", Framework: "go", Language: "go", PackageManager: "go",
		Port:       8080,
		InstallCmd: []string{"go", "mod", "download"},
		BuildCmd:   []string{"go", "build", "-o", "myapp", "."},
		StartCmd:   []string{"./myapp"},
	}
	df := renderDockerfile(svc)
	if !strings.Contains(df, "FROM golang:") {
		t.Errorf("go dockerfile should use golang builder; got:\n%s", df)
	}
	if !strings.Contains(df, "FROM alpine:latest") {
		t.Errorf("go dockerfile should use alpine runner; got:\n%s", df)
	}
	if !strings.Contains(df, "go mod download") {
		t.Errorf("go dockerfile should download deps; got:\n%s", df)
	}
	if !strings.Contains(df, "go build -o myapp") {
		t.Errorf("go dockerfile should build the binary; got:\n%s", df)
	}
	if !strings.Contains(df, "COPY --from=builder /app/myapp ./myapp") {
		t.Errorf("go dockerfile should copy the binary; got:\n%s", df)
	}
	if !strings.Contains(df, `CMD ["./myapp"]`) {
		t.Errorf("go dockerfile should CMD ./myapp; got:\n%s", df)
	}
	if !strings.Contains(df, "EXPOSE 8080") {
		t.Errorf("go dockerfile should EXPOSE 8080; got:\n%s", df)
	}
}

func TestRenderRustDockerfile(t *testing.T) {
	svc := detector.Service{
		ID: "s1", Framework: "rust", Language: "rust", PackageManager: "cargo",
		Port:     8080,
		BuildCmd: []string{"cargo", "build", "--release"},
		StartCmd: []string{"./target/release/my-rust-app"},
	}
	df := renderDockerfile(svc)
	if !strings.Contains(df, "FROM rust:") {
		t.Errorf("rust dockerfile should use rust builder; got:\n%s", df)
	}
	if !strings.Contains(df, "FROM debian:bookworm-slim") {
		t.Errorf("rust dockerfile should use debian runner; got:\n%s", df)
	}
	if !strings.Contains(df, "cargo build --release") {
		t.Errorf("rust dockerfile should build release; got:\n%s", df)
	}
	if !strings.Contains(df, "COPY --from=builder /app/target/release/my-rust-app") {
		t.Errorf("rust dockerfile should copy the binary; got:\n%s", df)
	}
	if !strings.Contains(df, `CMD ["./target/release/my-rust-app"]`) {
		t.Errorf("rust dockerfile should CMD the binary; got:\n%s", df)
	}
}

func TestBinaryNameExtraction(t *testing.T) {
	cases := []struct {
		startCmd []string
		want     string
	}{
		{[]string{"./app"}, "app"},
		{[]string{"./my-server"}, "my-server"},
		{[]string{"node", "index.js"}, "app"}, // fallback
		{[]string{}, "app"},
	}
	for _, c := range cases {
		svc := detector.Service{StartCmd: c.startCmd}
		got := binaryName(svc)
		if got != c.want {
			t.Errorf("binaryName(%v) = %s, want %s", c.startCmd, got, c.want)
		}
	}
}
