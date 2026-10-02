.PHONY: run test build

run:
	go run ./cmd/server $(ARGS)

test:
	go test -race ./...

build:
	go build -o bin/server ./cmd/server
