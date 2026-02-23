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
	gofmt -w *.go

clean:
	rm -f unifi-sync coverage.out coverage.html
