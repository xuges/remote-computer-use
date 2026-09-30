GO ?= go
VERSION ?= dev
NATIVE_FLAGS ?=
PREFIX ?= $(HOME)/.remote-computer-use

.PHONY: build test native native-input-test install integration-test integration-rdp-test clean
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

# Native runtime dependencies must remain installed in their original prefix.
install:
	install -d "$(DESTDIR)$(PREFIX)/bin"
	install -m 755 bin/remote-computer-ctl bin/remote-computer-use "$(DESTDIR)$(PREFIX)/bin/"
	install -m 755 bin/libremote-computer-*.so "$(DESTDIR)$(PREFIX)/bin/"

native-input-test:
	mkdir -p build
	$(CC) -Wall -Wextra -Wno-deprecated-declarations tests/vnc-input.c native/common.c -pthread -o build/test-vnc-input $$(pkg-config --cflags --libs gvnc-1.0)
	build/test-vnc-input

integration-test: build native-input-test
	mkdir -p build
	$(CC) -Wall -Wextra tests/desktop.c -o build/test-desktop $$(pkg-config --cflags --libs x11)
	python3 tests/integration.py

integration-rdp-test: build native-input-test
	mkdir -p build
	$(CC) -Wall -Wextra tests/desktop.c -o build/test-desktop $$(pkg-config --cflags --libs x11)
	python3 tests/integration.py --rdp

clean:
	rm -rf bin build
