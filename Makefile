.PHONY: build install test clean

GOBIN := $(shell go env GOBIN)
ifeq ($(GOBIN),)
GOBIN := $(shell go env GOPATH)/bin
endif

build:
	go build -o bin/quartr ./cmd/quartr

install:
	go install ./cmd/quartr
	@if [ -n "$$QUARTR_API_KEY" ]; then $(GOBIN)/quartr auth login --api-key "$$QUARTR_API_KEY"; fi

test:
	go test ./...

clean:
	rm -rf bin
