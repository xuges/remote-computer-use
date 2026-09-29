GO ?= go
VERSION ?= dev

.PHONY: build test native clean
build:
	mkdir -p bin
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags '-X github.com/xuges/remote-computer-use/internal/app.Version=$(VERSION)' -o bin/remote-computer-ctl ./cmd/remote-computer-ctl
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags '-X github.com/xuges/remote-computer-use/internal/app.Version=$(VERSION)' -o bin/remote-computer-use ./cmd/remote-computer-use

test:
	$(GO) test -race ./...
	$(GO) vet ./...

clean:
	rm -rf bin build
