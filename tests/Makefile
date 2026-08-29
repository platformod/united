.DEFAULT_GOAL := test
.SHELL := bash
.SHELLFLAGS := -eu -o pipefail -c

UNITED_BIN ?= ../dist/united
UNITED_TEST_PORT ?= 18080
GROUP_SLUG ?= integration
STATE_NAME ?= terraform
TF_HTTP_USERNAME ?= terraform
TF_HTTP_PASSWORD ?= integration-test-password

.PHONY: test

test: $(UNITED_BIN)
	@test -z "$${AWS_PROFILE:-}" || { echo "AWS_PROFILE is not supported by the PocketBase integration harness" >&2; exit 1; }
	@test -z "$${AWS_ACCESS_KEY_ID:-}" || { echo "AWS_ACCESS_KEY_ID is not supported by the PocketBase integration harness" >&2; exit 1; }
	@test -z "$${AWS_SECRET_ACCESS_KEY:-}" || { echo "AWS_SECRET_ACCESS_KEY is not supported by the PocketBase integration harness" >&2; exit 1; }
	@UNITED_BIN="$(UNITED_BIN)" UNITED_TEST_PORT="$(UNITED_TEST_PORT)" GROUP_SLUG="$(GROUP_SLUG)" STATE_NAME="$(STATE_NAME)" TF_HTTP_USERNAME="$(TF_HTTP_USERNAME)" TF_HTTP_PASSWORD="$(TF_HTTP_PASSWORD)" ./run.sh

$(UNITED_BIN): ../*.go
	$(MAKE) -C .. build
