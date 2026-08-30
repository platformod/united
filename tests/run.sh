#!/usr/bin/env bash
# SPDX-License-Identifier: MPL-2.0

set -euo pipefail

readonly united_bin=${UNITED_BIN:?UNITED_BIN must be set}
readonly api_url="http://127.0.0.1:8090"
readonly data_dir="$(dirname "$0")/tmp/pb_data"
readonly group_slug="integration"
readonly state_name="terraform"
readonly tf_http_username="integration-tf"
readonly tf_http_password="integration-test-password"
server_pid=""

cleanup() {
	if [[ -n "$server_pid" ]]; then
		kill "$server_pid" 2>/dev/null || true
		wait "$server_pid" 2>/dev/null || true
	fi
	rm -rf "$data_dir"
}
trap cleanup EXIT

start_server() {
	UNITED_STATE_MASTER_KEY="$UNITED_STATE_MASTER_KEY" "$united_bin" serve --dir="$data_dir" --http="127.0.0.1:8090" >"$data_dir/server.log" 2>&1 &
	server_pid=$!

	for _ in $(seq 1 30); do
		if ! kill -0 "$server_pid" 2>/dev/null; then
			wait "$server_pid" 2>/dev/null || true
			cat "$data_dir/server.log" >&2
			echo "PocketBase-backed United server exited before becoming ready" >&2
			exit 1
		fi
		if curl --fail --silent --show-error "$api_url/ping" >/dev/null; then
			return
		fi
		sleep 1
	done

	cat "$data_dir/server.log" >&2
	echo "PocketBase-backed United server did not become ready" >&2
	exit 1
}

stop_server() {
	kill "$server_pid"
	wait "$server_pid" || true
	server_pid=""
}

rm -rf "$data_dir"
mkdir -p "$data_dir"
cp -R "$(dirname "$0")/../test_pb_data/." "$data_dir"
export UNITED_STATE_MASTER_KEY
UNITED_STATE_MASTER_KEY=$(openssl rand -base64 32)
start_server

group_id=$("$(dirname "$0")/provision.sh" "$api_url" "$group_slug" "$tf_http_username" "$tf_http_password")
export TF_HTTP_ADDRESS="$api_url/state/$group_slug/$state_name"
export TF_HTTP_LOCK_ADDRESS="$TF_HTTP_ADDRESS"
export TF_HTTP_UNLOCK_ADDRESS="$TF_HTTP_ADDRESS"
export TF_HTTP_USERNAME="$tf_http_username"
export TF_HTTP_PASSWORD="$tf_http_password"
terraform init -reconfigure
terraform apply -auto-approve
terraform apply -var changer=bar -auto-approve
terraform state pull >/dev/null
terraform destroy -auto-approve
terraform state pull >/dev/null

lock_id="integration-lock"
lock_payload='{"Created":"","ID":"'"$lock_id"'","Info":"","Operation":"OperationTypeApply","Path":"","Version":"","Who":"integration-test"}'
curl --fail-with-body --silent --show-error --user "$TF_HTTP_USERNAME:$TF_HTTP_PASSWORD" --request LOCK --header 'Content-Type: application/json' --data "$lock_payload" "$TF_HTTP_ADDRESS"
lock_status=$(curl --silent --show-error --output "$data_dir/lock-conflict.json" --write-out '%{http_code}' --user "$TF_HTTP_USERNAME:$TF_HTTP_PASSWORD" --request LOCK --header 'Content-Type: application/json' --data '{"Created":"","ID":"competing-lock","Info":"","Operation":"OperationTypeApply","Path":"","Version":"","Who":"integration-test"}' "$TF_HTTP_ADDRESS")
test "$lock_status" = 423
# Terraform force-unlock remains excluded until its request shape is captured; see docs/terraform-force-unlock-investigation.md.
curl --fail --silent --show-error --user "$TF_HTTP_USERNAME:$TF_HTTP_PASSWORD" --request UNLOCK --header 'Content-Type: application/json' --data "$lock_payload" "$TF_HTTP_ADDRESS" >/dev/null
curl --fail --silent --show-error --user "$TF_HTTP_USERNAME:$TF_HTTP_PASSWORD" --request LOCK --header 'Content-Type: application/json' --data '{"Created":"","ID":"post-force-lock","Info":"","Operation":"OperationTypeApply","Path":"","Version":"","Who":"integration-test"}' "$TF_HTTP_ADDRESS" >/dev/null
curl --fail --silent --show-error --user "$TF_HTTP_USERNAME:$TF_HTTP_PASSWORD" --request UNLOCK --header 'Content-Type: application/json' --data '{"Created":"","ID":"post-force-lock","Info":"","Operation":"","Path":"","Version":"","Who":""}' "$TF_HTTP_ADDRESS" >/dev/null
curl --fail --silent --show-error --user "$TF_HTTP_USERNAME:$TF_HTTP_PASSWORD" --request DELETE "$TF_HTTP_ADDRESS" >/dev/null

stop_server
start_server
persisted_state_status=$(curl --silent --show-error --output /dev/null --write-out '%{http_code}' --user "$TF_HTTP_USERNAME:$TF_HTTP_PASSWORD" "$TF_HTTP_ADDRESS")
test "$persisted_state_status" = 404
