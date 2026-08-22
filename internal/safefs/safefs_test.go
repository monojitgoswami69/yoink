package safefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveStaysInsideRoot(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "ok.txt"), []byte("ok"), 0644)
	os.MkdirAll(filepath.Join(root, "sub"), 0755)
	os.WriteFile(filepath.Join(root, "sub/nested.txt"), []byte("nested"), 0644)

	r, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Resolve("ok.txt"); err != nil {
		t.Errorf("legit path rejected: %v", err)
	}
	if _, err := r.Resolve("sub/nested.txt"); err != nil {
		t.Errorf("legit nested path rejected: %v", err)
	}
	if _, err := r.Resolve("../etc/passwd"); err == nil {
		t.Error("escape via .. was allowed")
	}
	if _, err := r.Resolve("sub/../../etc/passwd"); err == nil {
		t.Error("escape via combined .. was allowed")
	}
	if _, err := r.Resolve("/etc/passwd"); err == nil {
		t.Error("absolute path was allowed")
	}
}

func TestReadCapsLargeFiles(t *testing.T) {
	root := t.TempDir()
	big := make([]byte, 2<<20) // 2 MiB
	for i := range big {
		big[i] = 'x'
	}
	os.WriteFile(filepath.Join(root, "big.txt"), big, 0644)

	r, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	content, err := r.Read("big.txt")
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(content)) > r.MaxSize+64 {
		t.Errorf("content not capped: %d bytes", len(content))
	}
}
