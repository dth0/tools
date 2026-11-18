export CGO_ENABLED := 0

BINDIR ?= $(HOME)/.local/bin

.PHONY: build install uninstall test

build:
	go build -o repos .

install:
	@mkdir -p $(BINDIR)
	go build -o $(BINDIR)/repos .

uninstall:
	rm -f $(BINDIR)/repos

test:
	go test ./...
