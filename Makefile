# Copyright © 2026 Michael Shields
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

.PHONY: all build test coverage-html clean lint lint-fix fmt tools deps help dev check pre-commit

BINARY_NAME=unifi-sync
COVERAGE_FILE=coverage.out
COVERAGE_HTML=coverage.html
GOCMD=go

all: deps fmt lint test build

help:
	@echo "Available targets:"
	@echo "  deps          - Download and verify dependencies"
	@echo "  build         - Build the binary"
	@echo "  test          - Run tests with race detector + 100% coverage check"
	@echo "  coverage-html - Generate HTML coverage report"
	@echo "  lint          - Run golangci-lint"
	@echo "  lint-fix      - Run golangci-lint with auto-fix"
	@echo "  fmt           - Format code with gofumpt and prettier"
	@echo "  clean         - Remove built binary and test artifacts"

tools:
	@bun install --cwd tools

deps: tools
	@echo "==> Downloading dependencies..."
	$(GOCMD) mod download
	$(GOCMD) mod tidy
	$(GOCMD) mod verify

build:
	@echo "==> Building $(BINARY_NAME)..."
	$(GOCMD) build -ldflags="-s -w" -o $(BINARY_NAME)

test:
	@echo "==> Running tests..."
	$(GOCMD) test -race -coverprofile=$(COVERAGE_FILE) ./...
	@LC_ALL=C awk 'NR>1{t+=$$2;if($$3>0)c+=$$2} \
	  END{printf "Coverage: %.1f%%\n",(t>0?100*c/t:0); \
	  if(c!=t){print "FAIL: coverage is not 100.0%";exit 1}}' $(COVERAGE_FILE)

coverage-html: test
	@echo "==> Generating HTML coverage report..."
	$(GOCMD) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo "Coverage report generated at $(COVERAGE_HTML)"

lint: actions-lock-verify-local
	@echo "==> Running golangci-lint..."
	$(GOCMD) tool golangci-lint run ./...

lint-fix:
	@echo "==> Running golangci-lint with auto-fix..."
	$(GOCMD) tool golangci-lint run --fix ./...

fmt: tools
	@echo "==> Formatting Go code..."
	$(GOCMD) tool gofumpt -w .
	@echo "==> Formatting Markdown, JSON, YAML files..."
	@git ls-files -z '*.md' '*.json' '*.json5' '*.yaml' '*.yml' | xargs -0 -I{} sh -c '! test -L "$$1" && test -f "$$1" && echo "$$1"' _ {} | xargs -I{} sh -c 'cd tools && bunx prettier --write "../$$1"' _ {}

clean:
	@echo "==> Cleaning..."
	@rm -f $(BINARY_NAME) $(COVERAGE_FILE) $(COVERAGE_HTML)
	@echo "Clean complete"

dev: fmt lint test build

check: deps fmt lint test

pre-commit: fmt lint test

.PHONY: actions-lock-install actions-lock actions-lock-verify-local actions-lock-verify

actions-lock-install:
	gh extension install github/gh-actions-lock --pin v0.4.1 --force

actions-lock:
	gh actions-lock --no-interactive

actions-lock-verify-local:
	gh actions-lock --verify-local

actions-lock-verify:
	gh actions-lock --verify
