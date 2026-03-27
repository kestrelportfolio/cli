# Development shortcuts.
#
# Ruby analogy: This is like a Rakefile with common tasks.
# `make build` is like `rake build`, `make install` is like `rake install`.

VERSION ?= dev

# Build for the current platform with version injected
build:
	go build -ldflags "-X github.com/kestrelportfolio/kestrel-cli/internal/api.Version=$(VERSION)" -o kestrel .

# Install to $GOPATH/bin (or $HOME/go/bin by default)
install:
	go install -ldflags "-X github.com/kestrelportfolio/kestrel-cli/internal/api.Version=$(VERSION)" .

# Test all packages
test:
	go test ./...

# Build snapshot binaries for all platforms (no publish)
release-snapshot:
	goreleaser release --snapshot --clean

# Clean build artifacts
clean:
	rm -f kestrel
	rm -rf dist/

.PHONY: build install test release-snapshot clean
