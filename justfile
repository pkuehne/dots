build:
    go build -o bin/dots ./cmd/dots

run *args:
    go run ./cmd/dots {{args}}

test:
    go test ./...

vet:
    go vet ./...

fmt:
    gofmt -w .

clean:
    rm -rf bin/
