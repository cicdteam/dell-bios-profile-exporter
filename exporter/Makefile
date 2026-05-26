MODULE := github.com/cicdteam/dell-bios-profile-exporter
BIN := dell-bios-profile-exporter
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

GOFUMPT_VERSION := v0.7.0
GOVULN_VERSION := v1.3.0
GOLANGCI_VERSION := v2.1.6

.PHONY: build test fmt vet lint vuln check

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o bin/$(BIN) ./cmd/$(BIN)

test:
	go test ./...

fmt:
	go run mvdan.cc/gofumpt@$(GOFUMPT_VERSION) -l -w .
	go run golang.org/x/tools/cmd/goimports@latest -local $(MODULE) -w .

vet:
	go vet ./...

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION) run --timeout 5m

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@$(GOVULN_VERSION) ./...

check: fmt vet lint vuln test
