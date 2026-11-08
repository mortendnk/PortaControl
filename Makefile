VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKG     := github.com/mortendnk/portacontrol/server/internal/version
LDFLAGS := -s -w -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE)

.PHONY: all build web web-dev test vet fmt demo fakeac bundle apk clean

all: vet test build

# The web UI is embedded into portacontrold, so build it first.
build: web
	cd server && CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o ../bin/ ./cmd/...

web:
	cd server/web && npm ci --no-audit --no-fund && npm run build

# Vite dev server with hot reload, proxying /api to a portacontrold on :8080.
web-dev:
	cd server/web && npm run dev

test:
	cd server && go test -count=1 ./...

vet:
	cd server && go vet ./...

fmt:
	cd server && gofmt -w .

# Server with a simulated PortaSplit on http://127.0.0.1:8080 (no hardware needed).
demo: build
	PORTACONTROL_DATA_DIR=./tmp-demo-data PORTACONTROL_LISTEN=127.0.0.1:8080 bin/portacontrold -config /dev/null -simulate

# A simulated PortaSplit on the LAN (TCP 6444, discovery UDP 6445/20086).
fakeac:
	cd server && go run ./cmd/portacontrol-fakeac

# Release bundle for deploy/proxmox/create-lxc.sh (PORTACONTROL_BUNDLE=...).
bundle: web
	rm -rf dist/bundle && mkdir -p dist/bundle
	cd server && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o ../dist/bundle/ ./cmd/portacontrold ./cmd/portacontrol-cli
	tar -C dist/bundle --mode=0755 --owner=0 --group=0 -czf dist/portacontrol-linux-amd64.tar.gz portacontrold portacontrol-cli
	cd dist && sha256sum portacontrol-linux-amd64.tar.gz > portacontrol-linux-amd64.tar.gz.sha256
	cp deploy/proxmox/create-lxc.sh dist/

apk:
	cd android && ./gradlew assembleRelease

clean:
	rm -rf bin dist tmp-demo-data
