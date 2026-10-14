DIST_DIR=dist
BIN_NAME=contributor-report

# Get version from git tag or hash, default to "dev"
VERSION := $(shell git describe --tags --exact-match HEAD 2>/dev/null || git rev-parse --short HEAD 2>/dev/null || echo "dev")

.PHONY: build
build: build-web build-cli

.PHONY: build-web
build-web:
	CGO_ENABLED=0 go build -v -ldflags "-X main.version=$(VERSION)" -o ${DIST_DIR}/contributor-report-web ./cmd/contributor-report-web

.PHONY: build-cli
build-cli:
	CGO_ENABLED=0 go build -v -ldflags "-X main.version=$(VERSION)" -o ${DIST_DIR}/contributor-report ./cmd/contributor-report

.PHONY: lint
lint:
	golangci-lint run --fix

.PHONY: test
test:
	go test -v ./...

.PHONY: test-coverage
test-coverage:
	go test -v -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -func=coverage.out

.PHONY: coverage-html
coverage-html: test-coverage
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

.PHONY: coverage
coverage:
	go test -cover ./...

.PHONY: release-binary
release-binary:
	CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build -v -ldflags "-X main.version=$(VERSION)" -o ${DIST_DIR}/${BIN_NAME} ./cmd/contributor-report

.PHONY: release
release:
	make BIN_NAME=contributor-report-darwin-amd64 GOOS=darwin GOARCH=amd64 release-binary
	make BIN_NAME=contributor-report-darwin-arm64 GOOS=darwin GOARCH=arm64 release-binary
	make BIN_NAME=contributor-report-linux-amd64 GOOS=linux GOARCH=amd64 release-binary
	make BIN_NAME=contributor-report-linux-arm64 GOOS=linux GOARCH=arm64 release-binary
	make BIN_NAME=contributor-report-windows-amd64.exe GOOS=windows GOARCH=amd64 release-binary

.PHONY: clean
clean:
	rm -rf $(DIST_DIR)
	rm -rf coverage.out coverage.html
