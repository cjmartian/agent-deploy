.PHONY: build fmt lint clean deps test

build:
	go build -o agent-deploy ./internal

fmt:
	gofmt -w -s .
	goimports -w .

lint:
	golangci-lint run ./...

clean:
	go clean

deps:
	go mod download
	go mod tidy

test:
	go test -v ./...