.PHONY: build test clean run

BINARY_NAME=dbseal

build:
	go build -o bin/$(BINARY_NAME) ./cmd/dbseal

test:
	go test -v ./internal/...

run:
	go run ./cmd/dbseal

clean:
	rm -rf bin/
