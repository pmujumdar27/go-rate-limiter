.PHONY: run build test clean deps docker-build docker-run help

help:
	@echo "Available commands:"
	@echo "  run         - Run the server locally"
	@echo "  build       - Build the binary"
	@echo "  test        - Run tests"
	@echo "  clean       - Clean build artifacts"
	@echo "  deps        - Download dependencies"
	@echo "  docker-build- Build Docker image"
	@echo "  docker-run  - Run with Docker"

run:
	go run cmd/server/main.go

build:
	go build -o bin/server cmd/server/main.go

test:
	go test ./...

clean:
	rm -rf bin/
	go clean

deps:
	go mod download
	go mod tidy

docker-build:
	docker build -t go-rate-limiter .

docker-run:
	docker run -p 8080:8080 go-rate-limiter