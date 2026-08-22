// Package pydeps maps Python dependency names to the Debian/Ubuntu system
// packages their C-extension builds require. This lets the generator emit a
// single `apt-get install` line up front so wheels that compile from source
// (or have no linux wheel for the target arch) don't break the build.
//
// The map is conservative: when a dependency has a prebuilt manylinux wheel
// (the common case on x86_64), the extra -dev packages are simply unused.
// When no wheel exists (python 3.13, arm64, or a source-only package), the
// headers are already present and the build succeeds.
//
// Pure-Python packages (uvicorn, gunicorn, pymysql, etc.) are deliberately
// absent from both maps so a pure-Python project yields nil and stays lean.
package pydeps

import (
	"sort"
	"strings"
)

// AptPackages returns the debian apt packages that should be installed
// before `pip install`/`poetry install`/`uv sync` for the given project
// dependencies. Returns nil when none of the dependencies are known to need
// system headers or a compiler, so pure-Python projects stay lean.
//
// build-essential (gcc/g++/make) is added whenever any native dependency is
// present, since compiling a C extension needs a compiler.
func AptPackages(pythonDeps []string) []string {
	set := map[string]bool{}
	native := false
	for _, d := range pythonDeps {
		d = strings.ToLower(d)
		if pkgs, ok := depApt[d]; ok {
			native = true
			for _, p := range pkgs {
				set[p] = true
			}
		}
		if needsCompiler[d] {
			native = true
		}
	}
	if !native {
		return nil
	}
	// Every C-extension build needs gcc + make. build-essential pulls both
	// plus g++; only added when at least one native dep is present.
	set["build-essential"] = true
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// depApt maps a Python package name (lowercased, no extras/versions) to the
// Debian -dev packages its source build needs. Entries here always pull
// build-essential as well (via AptPackages).
var depApt = map[string][]string{
	// PostgreSQL drivers
	"psycopg":         {"libpq-dev"},
	"psycopg2":        {"libpq-dev"},
	"psycopg2-binary": {"libpq-dev"},
	"psycopg2cffi":    {"libpq-dev"},
	"asyncpg":         {"libpq-dev"},
	"pgvector":        {"libpq-dev"},

	// MySQL drivers
	"mysqlclient": {"default-libmysqlclient-dev", "pkg-config"},
	"pymssql":     {"freetds-dev"},

	// Crypto / TLS
	"cryptography": {"libffi-dev", "libssl-dev"},
	"pyOpenSSL":    {"libssl-dev"},
	"bcrypt":       {"libffi-dev"},
	"cffi":         {"libffi-dev"},
	"paramiko":     {"libffi-dev", "libssl-dev"},
	"pysftp":       {"libffi-dev", "libssl-dev"},
	"python-ldap":  {"libldap2-dev", "libsasl2-dev"},

	// XML
	"lxml":  {"libxml2-dev", "libxslt1-dev"},
	"uwsgi": {"libpcre3-dev", "libxml2-dev"},

	// Imaging
	"pillow": {"libjpeg-dev", "zlib1g-dev"},
	"pil":    {"libjpeg-dev", "zlib1g-dev"},

	// GIS
	"shapely":   {"libgeos-dev"},
	"fiona":     {"libgdal-dev"},
	"rasterio":  {"libgdal-dev"},
	"pyproj":    {"libproj-dev"},
	"geopandas": {"libgeos-dev", "libgdal-dev"},

	// Audio / media
	"pyaudio":   {"portaudio19-dev"},
	"soundfile": {"libsndfile1-dev"},
	"av":        {"libavformat-dev", "libavcodec-dev", "libavdevice-dev"},

	// Native / IPC / data
	"pyzmq":            {"libzmq3-dev"},
	"h5py":             {"libhdf5-dev"},
	"tables":           {"libhdf5-dev"},
	"netcdf4":          {"libnetcdf-dev"},
	"cassandra-driver": {"libev-dev"},
	"pylibmc":          {"libmemcached-dev"},
}

// needsCompiler lists packages whose source builds need a compiler but no
// specific -dev headers. Their manylinux wheels usually exist, but when a
// wheel is missing for the target arch/python they must build from source —
// so we install build-essential up front rather than fail the build.
var needsCompiler = map[string]bool{
	"greenlet":     true,
	"grpcio":       true,
	"grpcio-tools": true,
	"orjson":       true,
	"msgpack":      true,
	"uvloop":       true,
	"regex":        true,
	"watchdog":     true,
	"twisted":      true,
	"markupsafe":   true,
}
