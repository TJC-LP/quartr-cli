.PHONY: build test clean

build:
	go build -o bin/quartr ./cmd/quartr

test:
	go test ./...

clean:
	rm -rf bin
