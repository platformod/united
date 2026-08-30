.DEFAULT_GOAL := test
.SHELL := bash
.SHELLFLAGS := -eu -o pipefail -c

UNITED_BIN ?= ../dist/united

GROUP_SLUG ?= integration
STATE_NAME ?= terraform

.PHONY: test

test: $(UNITED_BIN)
	@test -z "$${AWS_PROFILE:-}" || { echo "AWS_PROFILE is not supported by the PocketBase integration harness" >&2; exit 1; }
	@test -z "$${AWS_ACCESS_KEY_ID:-}" || { echo "AWS_ACCESS_KEY_ID is not supported by the PocketBase integration harness" >&2; exit 1; }
	@test -z "$${AWS_SECRET_ACCESS_KEY:-}" || { echo "AWS_SECRET_ACCESS_KEY is not supported by the PocketBase integration harness" >&2; exit 1; }
	@test -z "$${ADMIN_EMAIL:-}" || { echo "ADMIN_EMAIL must not be set; the integration harness generates it at runtime" >&2; exit 1; }
	@test -z "$${ADMIN_PASSWORD:-}" || { echo "ADMIN_PASSWORD must not be set; the integration harness generates it at runtime" >&2; exit 1; }
	@UNITED_BIN="$(UNITED_BIN)" GROUP_SLUG="$(GROUP_SLUG)" STATE_NAME="$(STATE_NAME)" ./run.sh

$(UNITED_BIN): ../*.go
	$(MAKE) -C .. build
