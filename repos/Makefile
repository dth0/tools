export CGO_ENABLED := 0

.PHONY: build install test

build:
	go build -o repos .

install:
	go install .

test:
	go test ./...
