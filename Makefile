ifeq (,$(shell go env GOBIN))
	GOBIN=$(shell go env GOPATH)/bin
else
	GOBIN=$(shell go env GOBIN)
endif

ifeq ($(OS),Windows_NT)
	DETECTED_OS := Windows
else
	DETECTED_OS := $(shell uname -s)
endif

cli:
	GOOS=darwin GOARCH=arm64 go build -ldflags "-w -s" -o bin/envoy ./cmd/envoy
	GOOS=linux GOARCH=amd64 go build -ldflags "-w -s" -o bin/envoy-linux ./cmd/envoy
	GOOS=windows GOARCH=amd64 go build -ldflags "-w -s" -o bin/envoy.exe ./cmd/envoy

install: cli
ifeq ($(DETECTED_OS),Windows)
	cp bin/envoy.exe $(GOBIN)/envoy.exe
else ifeq ($(DETECTED_OS),Darwin)
	cp bin/envoy $(GOBIN)/envoy
else
	cp bin/envoy-linux $(GOBIN)/envoy
endif
