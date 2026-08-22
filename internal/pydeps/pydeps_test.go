package pydeps

import (
	"sort"
	"testing"
)

func TestAptPackagesForPostgresDriver(t *testing.T) {
	got := AptPackages([]string{"fastapi", "psycopg", "uvicorn"})
	if got == nil {
		t.Fatal("expected non-nil apt packages for psycopg")
	}
	if !contains(got, "libpq-dev") {
		t.Errorf("psycopg must pull libpq-dev; got %v", got)
	}
	if !contains(got, "build-essential") {
		t.Errorf("native dep must pull build-essential; got %v", got)
	}
}

func TestAptPackagesForMySQLClient(t *testing.T) {
	got := AptPackages([]string{"mysqlclient"})
	if !contains(got, "default-libmysqlclient-dev") || !contains(got, "pkg-config") {
		t.Errorf("mysqlclient missing its dev deps; got %v", got)
	}
}

func TestAptPackagesNilForPurePython(t *testing.T) {
	if got := AptPackages([]string{"fastapi", "uvicorn", "pydantic", "requests"}); got != nil {
		t.Errorf("pure-python deps must yield nil apt packages; got %v", got)
	}
}

func TestAptPackagesNilForEmpty(t *testing.T) {
	if got := AptPackages(nil); got != nil {
		t.Errorf("empty deps must yield nil; got %v", got)
	}
}

func TestAptPackagesDedupes(t *testing.T) {
	// psycopg + psycopg2 + asyncpg all want libpq-dev — it should appear once.
	got := AptPackages([]string{"psycopg", "psycopg2", "asyncpg"})
	count := 0
	for _, p := range got {
		if p == "libpq-dev" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("libpq-dev should appear once, got %d (%v)", count, got)
	}
}

func TestAptPackagesCaseInsensitive(t *testing.T) {
	got := AptPackages([]string{"Psycopg", "CRYPTOGRAPHY"})
	if got == nil {
		t.Fatal("expected apt packages for mixed-case dep names")
	}
	if !contains(got, "libpq-dev") || !contains(got, "libffi-dev") {
		t.Errorf("case-insensitive match failed; got %v", got)
	}
}

func contains(haystack []string, needle string) bool {
	sorted := append([]string(nil), haystack...)
	sort.Strings(sorted)
	// tiny linear search is fine for these small slices
	for _, s := range sorted {
		if s == needle {
			return true
		}
	}
	return false
}
