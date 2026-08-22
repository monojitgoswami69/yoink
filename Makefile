.PHONY: all build test vet fmt clean install run

BIN       ?= yoink
PREFIX    ?= /usr/local
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo v0.2.0)
BUILDTIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS   := -s -w -X yoink/cmd.Version=$(VERSION) -X yoink/cmd.BuildTime=$(BUILDTIME)

all: build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN) .

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -s -w .

run: build
	./$(BIN) help

clean:
	rm -f $(BIN)

install: build
	install -d $(DESTDIR)$(PREFIX)/bin
	install -m 0755 $(BIN) $(DESTDIR)$(PREFIX)/bin/$(BIN)
