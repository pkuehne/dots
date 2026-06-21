# List available recipes (also runs when you call `just` with no args).
help:
    @just --list

# Build the dots binary to bin/dots.
build:
    go build -o bin/dots ./cmd/dots

# Compile everything without producing a binary (fast sanity check).
check:
    go build ./...

# Run dots from source, e.g. `just run apply --dry-run`.
run *args:
    go run ./cmd/dots {{args}}

# Build and install dots into your GOBIN / GOPATH bin.
install:
    go install ./cmd/dots

# Run the unit test suite.
test:
    go test ./...

# Run the end-to-end tests (requires `age` on PATH for the secrets round-trip).
test-e2e:
    go test -tags e2e ./cmd/dots/...

# Run all tests, unit and e2e.
test-all: test test-e2e

# Run tests with coverage and open an HTML report.
cover:
    go test -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out

# Format all Go source in place.
fmt:
    gofmt -w .

# Fail if any file is not gofmt-clean (mirrors CI).
fmt-check:
    @unformatted=$(gofmt -l .); \
    if [ -n "$unformatted" ]; then \
        echo "These files are not gofmt-clean:"; \
        echo "$unformatted"; \
        exit 1; \
    fi

# Run go vet.
vet:
    go vet ./...

# Tidy and verify module dependencies.
tidy:
    go mod tidy

# Run the same checks CI does before you push.
ci: fmt-check vet test test-e2e

# Remove build artifacts.
clean:
    rm -rf bin/ coverage.out
