.PHONY: help tidy build run test lint clean docker-up docker-down

BIN_DIR := bin
BIN_NAME := xiaozhao-server
CONFIG ?= configs/config.yaml

help:
	@echo "make tidy        - go mod tidy"
	@echo "make build       - build binary to $(BIN_DIR)/$(BIN_NAME)"
	@echo "make run         - run server with $(CONFIG)"
	@echo "make test        - run unit tests"
	@echo "make docker-up   - start postgres + redis via docker compose"
	@echo "make docker-down - stop docker compose services"

tidy:
	go mod tidy

build:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o $(BIN_DIR)/$(BIN_NAME) ./cmd/server

run:
	go run ./cmd/server --config $(CONFIG)

test:
	go test ./... -race -count=1

clean:
	rm -rf $(BIN_DIR)

docker-up:
	docker compose -f scripts/docker-compose.yaml up -d

docker-down:
	docker compose -f scripts/docker-compose.yaml down
