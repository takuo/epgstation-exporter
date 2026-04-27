API_URL ?= http://localhost:8888/api
OAPI_CODEGEN = go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest
BINARY = epgstation-exporter

.PHONY: all generate build test clean fetch-api-doc

all: build

fetch-api-doc:
	curl -sSf -o pkg/epgstation/gen/api-doc.json $(API_URL)/docs

pkg/epgstation/api.gen.go: pkg/epgstation/gen/api-doc.json
	go generate ./pkg/epgstation/gen

generate: pkg/epgstation/api.gen.go

build: generate
	go build -o $(BINARY) ./cmd/epgstation-exporter/

test:
	go test ./...

clean:
	rm -f $(BINARY)
	rm -f pkg/epgstation/api.gen.go
