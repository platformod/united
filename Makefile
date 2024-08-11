.PHONY: help
.DEFAULT_GOAL := help
.SHELLFLAGS := -c
.SHELL := bash

run = ~/go/bin/air
ifdef CI
	run = ./dist/united &
endif

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

build: *.go  ## Builds the program
	go build -o dist/united

devprep: ## Installs all dev tools you need
	brew bundle install
	tfenv install
	pre-commit install
	go install github.com/air-verse/air@latest

runtime: ## Run Docker deps
	docker compose up --quiet-pull -d
	sleep 2

setup-localstack: ## Setup up localstack
	AWS_PROFILE="localstack" aws s3 ls united-test || aws s3 mb s3://united-test
	AWS_PROFILE="localstack" aws kms list-aliases | jq '.Aliases[] | select(.AliasName=="alias/united-test")' | grep united-test || aws kms create-alias --alias-name alias/united-test --target-key-id $$(aws kms create-key | jq -r '.KeyMetadata.KeyId') | cat

down: ## Down compose
	docker compose down

run: build runtime setup-localstack  ## Run united devmode
	DEV="true" AWS_PROFILE="localstack" BUCKET="united-test" KEY_ARN="alias/united-test" AUTH_URL="http://localhost:8085:/united-test" $(run)

test: ## Run tests in tests/ dir
	TF_HTTP_USERNAME=foo TF_HTTP_PASSWORD=f00f00f00 $(MAKE) -C tests
