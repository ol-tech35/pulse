.PHONY: build test vet fmt

build:
	go build -trimpath -o bin/pulse ./cmd/pulse

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w cmd internal
