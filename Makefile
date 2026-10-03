BIN     := bin/moodle-mcp
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build test setup-test cover lint golden fuzz dev install-skills uninstall-skills

build:
	go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(BIN) ./cmd/moodle-mcp

test:
	go test -race -count=1 ./...

setup-test:  # ./setup.sh end to end against the fake Moodle and a stub claude CLI
	dev/setup_test.sh

cover:
	go test -race -count=1 -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

lint:
	go vet ./...
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run ./...
	shellcheck setup.sh dev/*.sh

FUZZTIME ?= 30s

fuzz:  # every fuzz target for $(FUZZTIME); failing inputs land in testdata/fuzz of the package
	go test -run '^$$' -fuzz '^FuzzStripHTML$$' -fuzztime $(FUZZTIME) ./internal/textfmt
	go test -run '^$$' -fuzz '^FuzzTruncate$$' -fuzztime $(FUZZTIME) ./internal/textfmt
	go test -run '^$$' -fuzz '^FuzzText$$' -fuzztime $(FUZZTIME) ./internal/extract
	go test -run '^$$' -fuzz '^FuzzDecode$$' -fuzztime $(FUZZTIME) ./internal/moodle
	go test -run '^$$' -fuzz '^FuzzToWebserviceURL$$' -fuzztime $(FUZZTIME) ./internal/moodle
	go test -run '^$$' -fuzz '^FuzzSearchInput$$' -fuzztime $(FUZZTIME) ./internal/study

golden:
	go test ./internal/tools/ -update

dev:  # fake Moodle and the mcpcall CLI, for trying skills without the real site
	go build -o bin/fakemoodle ./dev/fakemoodle
	go build -o bin/mcpcall ./dev/mcpcall

SKILLS_DIR ?= $(HOME)/.claude/skills

install-skills:
	mkdir -p $(SKILLS_DIR)
	for s in skills/*/; do n=$$(basename $$s); ln -sfn "$(CURDIR)/skills/$$n" "$(SKILLS_DIR)/$$n"; echo "installed $$n"; done

uninstall-skills:
	for s in skills/*/; do n=$$(basename $$s); [ -L "$(SKILLS_DIR)/$$n" ] && rm "$(SKILLS_DIR)/$$n" && echo "removed $$n"; done; true
