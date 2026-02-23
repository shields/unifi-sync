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

.PHONY: all build test coverage lint lint-fix fmt clean

all: fmt lint test build

build:
	go build -o unifi-sync

test:
	go test -race -coverprofile=coverage.out ./...
	@coverage=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}'); \
	echo "Coverage: $$coverage"; \
	if [ "$$coverage" != "100.0%" ]; then \
		echo "FAIL: coverage is $$coverage, want 100.0%"; \
		exit 1; \
	fi

coverage: test
	go tool cover -html=coverage.out -o coverage.html

lint:
	go tool golangci-lint run ./...

lint-fix:
	go tool golangci-lint run --fix ./...

fmt:
	go tool gofumpt -w .

clean:
	rm -f unifi-sync coverage.out coverage.html
