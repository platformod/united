#!/usr/bin/env bash
# SPDX-License-Identifier: MPL-2.0

set -euo pipefail

united_bin=${UNITED_BIN:?UNITED_BIN must be set}
script_dir=$(cd "$(dirname "$0")" && pwd)
run_script="$script_dir/run.sh"
provision_script="$script_dir/provision.sh"

require_contains() {
	local file=$1
	local expected=$2

	grep -Fq -- "$expected" "$file" || {
		echo "expected $file to contain: $expected" >&2
		exit 1
	}
}

require_absent() {
	local file=$1
	local unexpected=$2

	if grep -Fq -- "$unexpected" "$file"; then
		echo "expected $file not to contain: $unexpected" >&2
		exit 1
	fi
}

assert_runtime_secret() {
	local file=$1
	local name=$2
	local generator=$3

	awk -v name="$name" -v generator="$generator" '
		index($0, name "=") {
			if ($0 == name "=" generator) {
				generated++
			} else if (index($0, name "=\"$" name "\"") == 0) {
				invalid++
			}
		}
		END { exit generated == 1 && invalid == 0 ? 0 : 1 }
	' "$file" || {
		echo "expected $name to be generated once without a static/default fallback in $file" >&2
		exit 1
	}
}

assert_protected_curl_has_bearer() {
	local file=$1
	local endpoint=$2

	awk -v endpoint="$endpoint" '
		function validate() {
			if (has_endpoint) {
				found++
				if (!has_bearer) {
					invalid++
				}
			}
		}
		/curl / {
			if (in_curl) {
				validate()
			}
			in_curl = 1
			has_bearer = 0
			has_endpoint = 0
		}
		in_curl {
			if (index($0, "Authorization: Bearer") > 0) {
				has_bearer = 1
			}
			if (index($0, endpoint) > 0) {
				has_endpoint = 1
			}
			if ($0 !~ /\\[[:space:]]*$/) {
				validate()
				in_curl = 0
			}
		}
		END {
			if (in_curl) {
				validate()
			}
			exit found == 1 && invalid == 0 ? 0 : 1
		}
	' "$file" || {
		echo "expected protected curl invocation for $endpoint to include a bearer header" >&2
		exit 1
	}
}

assert_test_command_is_unknown() {
	local command=$1
	local output

	output=$(mktemp)
	trap 'rm -f "$output"' RETURN
	if UNITED_STATE_MASTER_KEY=$(openssl rand -base64 32) "$united_bin" "$command" --dir="$(mktemp -d)" > /dev/null 2>"$output"; then
		echo "expected $command to be unavailable" >&2
		exit 1
	fi
	require_contains "$output" "unknown command \"$command\""
}

assert_test_command_is_unknown test-provision
assert_test_command_is_unknown test-inspect

require_contains "$run_script" 'TF_HTTP_PASSWORD=$(openssl rand -base64 24)'
require_contains "$run_script" 'ADMIN_PASSWORD=$(openssl rand -base64 24)'
assert_runtime_secret "$run_script" UNITED_STATE_MASTER_KEY '$(openssl rand -base64 32)'
require_contains "$provision_script" 'password=${TF_HTTP_PASSWORD:?TF_HTTP_PASSWORD must be set}'
require_absent "$run_script" 'correct horse'
require_absent "$provision_script" 'correct horse'
require_absent "$run_script" 'owner@example.test'
require_absent "$provision_script" 'owner@example.test'

require_contains "$provision_script" '$base_url/api/collections/$collection/auth-with-password'
require_contains "$run_script" '$api_url/api/collections/_superusers/auth-with-password'
assert_protected_curl_has_bearer "$provision_script" '$base_url/api/collections/users/records'
assert_protected_curl_has_bearer "$provision_script" '$base_url/api/collections/groups/records'
assert_protected_curl_has_bearer "$run_script" '$api_url/api/collections/states/records'
assert_protected_curl_has_bearer "$run_script" '$api_url/api/collections/statefiles/records'
