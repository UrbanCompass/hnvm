.PHONY: build test lint parity

# The bootstrap shims in bin/ run dist/hnvm when it exists.
build:
	go build -trimpath -ldflags "-X main.version=$$(git describe --tags --always --dirty)" -o dist/hnvm ./cmd/hnvm

test:
	go test -race ./...

lint:
	go vet ./...
	test -z "$$(gofmt -l .)"
	shellcheck bin/* lib/hnvm/*.sh install.sh uninstall.sh

# The jest suite runs bin/*, which picks up dist/hnvm.
parity: build
	cd test && npm test
