GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=aiconnect
BINARY_PATH=./cmd/aiconnect
BUILD_DIR=./build

# Container settings
CONTAINER_ENGINE ?= podman
IMAGE_NAME ?= aiconnect
IMAGE_TAG ?= latest

.PHONY: all build clean test run install container-build container-run container-push

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

# Container targets (compatibile con Podman e Docker)
container-build:
	$(CONTAINER_ENGINE) build -t $(IMAGE_NAME):$(IMAGE_TAG) -f Containerfile .

container-run:
	$(CONTAINER_ENGINE) run -d --name $(BINARY_NAME) \
		-p 443:443 -p 9090:9090 \
		-v ./config.example.yaml:/etc/aiconnect/config.yaml:ro \
		$(IMAGE_NAME):$(IMAGE_TAG)

container-stop:
	$(CONTAINER_ENGINE) stop $(BINARY_NAME) || true
	$(CONTAINER_ENGINE) rm $(BINARY_NAME) || true

container-push:
	$(CONTAINER_ENGINE) push $(IMAGE_NAME):$(IMAGE_TAG)

# Alias per Docker users
docker-build: CONTAINER_ENGINE=docker
docker-build: container-build

docker-run: CONTAINER_ENGINE=docker
docker-run: container-run

docker-stop: CONTAINER_ENGINE=docker
docker-stop: container-stop
