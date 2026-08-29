.PHONY: help build devprep run test
.DEFAULT_GOAL := help
.SHELLFLAGS := -c
.SHELL := bash

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

build: *.go  ## Build the program
	go build -buildvcs=false -o dist/united

devprep: ## Install development tools
	brew bundle install
	tfenv install
	pre-commit install
	go install github.com/air-verse/air@latest

run: build  ## Run United with PocketBase data in ./pb_data
	@test -n "$${UNITED_STATE_MASTER_KEY:-}" || { echo "UNITED_STATE_MASTER_KEY must be a base64-encoded 32-byte key" >&2; exit 1; }
	UNITED_STATE_MASTER_KEY="$${UNITED_STATE_MASTER_KEY}" ~/go/bin/air

test: ## Run the standalone Terraform integration harness
	$(MAKE) -C tests test
