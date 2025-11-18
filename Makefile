GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=aiconnect
BINARY_PATH=./cmd/aiconnect
BUILD_DIR=./build

.PHONY: all build clean test run install

all: test build

build:
	$(GOBUILD) -o $(BINARY_NAME) $(BINARY_PATH)

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME)-linux-amd64 $(BINARY_PATH)

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME)-linux-amd64

test:
	$(GOTEST) -v ./...

run:
	$(GOBUILD) -o $(BINARY_NAME) $(BINARY_PATH)
	AICONNECT_CONFIG=./config.example.yaml ./$(BINARY_NAME)

install: build
	sudo bash deployment/install.sh

deps:
	$(GOGET) -v ./...
	$(GOCMD) mod tidy

fmt:
	$(GOCMD) fmt ./...

vet:
	$(GOCMD) vet ./...
