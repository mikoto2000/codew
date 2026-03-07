# Build the Go project
build:
	go build -o build/codew main.go

# Run linting checks using golint
fmt:
	go fmt ./...

# Run tests
unittests:
	go test ./...

# Default target
all: build lint unittests