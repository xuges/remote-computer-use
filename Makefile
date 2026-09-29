GO ?= go
VERSION ?= dev
NATIVE_FLAGS ?=

.PHONY: build test native integration-test integration-rdp-test clean
build:
	mkdir -p bin
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags '-X github.com/xuges/remote-computer-use/internal/app.Version=$(VERSION)' -o bin/remote-computer-ctl ./cmd/remote-computer-ctl
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags '-X github.com/xuges/remote-computer-use/internal/app.Version=$(VERSION)' -o bin/remote-computer-use ./cmd/remote-computer-use

test:
	$(GO) test -race ./...
	$(GO) vet ./...

native:
	cmake -S native -B build/native -DCMAKE_BUILD_TYPE=RelWithDebInfo -DCMAKE_LIBRARY_OUTPUT_DIRECTORY=$(CURDIR)/bin $(NATIVE_FLAGS)
	cmake --build build/native --parallel 4

integration-test: build
	mkdir -p build
	$(CC) -Wall -Wextra tests/desktop.c -o build/test-desktop $$(pkg-config --cflags --libs x11)
	python3 tests/integration.py

integration-rdp-test: build
	mkdir -p build
	$(CC) -Wall -Wextra tests/desktop.c -o build/test-desktop $$(pkg-config --cflags --libs x11)
	python3 tests/integration.py --rdp

clean:
	rm -rf bin build
