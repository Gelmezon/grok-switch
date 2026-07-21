APP     := grok-switch
DIST    := dist
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/Gelmezon/grok-switch/internal/cli.Version=$(VERSION)

.PHONY: test race vet build release clean install

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

build:
	mkdir -p $(DIST)
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" \
		-o $(DIST)/$(APP) ./cmd/$(APP)

release: test vet
	mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-trimpath -ldflags="$(LDFLAGS)" \
		-o $(DIST)/$(APP)-linux-amd64 ./cmd/$(APP)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
		-trimpath -ldflags="$(LDFLAGS)" \
		-o $(DIST)/$(APP)-linux-arm64 ./cmd/$(APP)
	cd $(DIST) && sha256sum $(APP)-linux-* > SHA256SUMS

install: build
	install -Dm755 $(DIST)/$(APP) $(DESTDIR)/usr/local/bin/$(APP)

clean:
	rm -rf $(DIST)
