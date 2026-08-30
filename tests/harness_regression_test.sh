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
require_contains "$provision_script" 'password=${TF_HTTP_PASSWORD:?TF_HTTP_PASSWORD must be set}'
require_absent "$run_script" 'correct horse'
require_absent "$provision_script" 'correct horse'
require_absent "$run_script" 'owner@example.test'
require_absent "$provision_script" 'owner@example.test'

require_contains "$provision_script" 'Authorization: Bearer $admin_token'
require_contains "$provision_script" 'Authorization: Bearer $owner_token'
require_contains "$provision_script" '$base_url/api/collections/$collection/auth-with-password'
require_contains "$provision_script" '$base_url/api/collections/users/records'
require_contains "$provision_script" '$base_url/api/collections/groups/records'
require_contains "$run_script" 'Authorization: Bearer $admin_token'
require_contains "$run_script" '$api_url/api/collections/_superusers/auth-with-password'
require_contains "$run_script" '$api_url/api/collections/states/records'
require_contains "$run_script" '$api_url/api/collections/statefiles/records'
