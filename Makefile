.PHONY: build test run verify fmt vet install uninstall

PREFIX ?= /usr/local

build:
	go build -o sphragis ./cmd/sphragis

test:
	go test ./...

run: build
	./sphragis serve

verify: build
	./sphragis verify $(LOG)

fmt:
	go fmt ./...

vet:
	go vet ./...

install: build
	install -d $(PREFIX)/bin
	install -m 0755 sphragis $(PREFIX)/bin/sphragis

uninstall:
	rm -f $(PREFIX)/bin/sphragis
