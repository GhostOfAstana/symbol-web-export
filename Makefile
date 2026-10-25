.PHONY: run test audit build
run:
	go run ./cmd/server
test:
	go test ./...
audit:
	test -z "$$(gofmt -l .)"
	go vet ./...
	go test -race -cover ./...
build:
	go build -trimpath -o server ./cmd/server
