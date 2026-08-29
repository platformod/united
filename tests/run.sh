#!/usr/bin/env bash
# SPDX-License-Identifier: MPL-2.0

set -euo pipefail

united_bin=${UNITED_BIN:?UNITED_BIN must be set}
group_slug=${GROUP_SLUG:?GROUP_SLUG must be set}
state_name=${STATE_NAME:?STATE_NAME must be set}
TF_HTTP_USERNAME="terraform-$RANDOM-$RANDOM"
TF_HTTP_PASSWORD=$(openssl rand -base64 24)
export TF_HTTP_USERNAME TF_HTTP_PASSWORD

data_dir=$(mktemp -d "${TMPDIR:-/tmp}/united-pocketbase-test.XXXXXX")
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
	for _ in $(seq 1 10); do
		port=$((20000 + RANDOM % 30000))
		UNITED_STATE_MASTER_KEY="$UNITED_STATE_MASTER_KEY" "$united_bin" serve --dir="$data_dir" --http="127.0.0.1:$port" >"$data_dir/server.log" 2>&1 &
		server_pid=$!

		for _ in $(seq 1 30); do
			if ! kill -0 "$server_pid" 2>/dev/null; then
				wait "$server_pid" 2>/dev/null || true
				server_pid=""
				break
			fi
			if curl --fail --silent --show-error "http://127.0.0.1:$port/ping" >/dev/null; then
				return
			fi
			sleep 1
		done

		if [[ -n "$server_pid" ]]; then
			kill "$server_pid" 2>/dev/null || true
			wait "$server_pid" 2>/dev/null || true
			server_pid=""
		fi
	done

	cat "$data_dir/server.log" >&2
	echo "PocketBase-backed United server did not become ready after 10 attempts" >&2
	exit 1
}

stop_server() {
	kill "$server_pid"
	wait "$server_pid" || true
	server_pid=""
}

inspect_state() {
	"$united_bin" test-inspect --dir="$data_dir" --group-slug="$group_slug" --state-name="$state_name"
}

export UNITED_STATE_MASTER_KEY
UNITED_STATE_MASTER_KEY=$(openssl rand -base64 32)
start_server
export TF_HTTP_ADDRESS="http://127.0.0.1:$port/state/$group_slug/$state_name"
export TF_HTTP_LOCK_ADDRESS="$TF_HTTP_ADDRESS"
export TF_HTTP_UNLOCK_ADDRESS="$TF_HTTP_ADDRESS"
"$(dirname "$0")/provision.sh" "$united_bin" "$data_dir" "owner@example.test" "$group_slug" "$TF_HTTP_USERNAME" "$TF_HTTP_PASSWORD"
terraform init -reconfigure
terraform apply -lock=false -auto-approve
terraform apply -lock=false -var changer=bar -auto-approve
terraform state pull >/dev/null
terraform destroy -lock=false -auto-approve
terraform state pull >/dev/null

inspection=$(inspect_state)
echo "$inspection" | grep -q '"states":1'
echo "$inspection" | grep -Eq '"versions":[3-9][0-9]*'
echo "$inspection" | grep -q '"deleted":false'

lock_id="integration-lock"
lock_payload='{"Created":"","ID":"'"$lock_id"'","Info":"","Operation":"OperationTypeApply","Path":"","Version":"","Who":"integration-test"}'
curl --fail-with-body --silent --show-error --user "$TF_HTTP_USERNAME:$TF_HTTP_PASSWORD" --request LOCK --header 'Content-Type: application/json' --data "$lock_payload" "$TF_HTTP_ADDRESS"
lock_status=$(curl --silent --show-error --output "$data_dir/lock-conflict.json" --write-out '%{http_code}' --user "$TF_HTTP_USERNAME:$TF_HTTP_PASSWORD" --request LOCK --header 'Content-Type: application/json' --data '{"Created":"","ID":"competing-lock","Info":"","Operation":"OperationTypeApply","Path":"","Version":"","Who":"integration-test"}' "$TF_HTTP_ADDRESS")
test "$lock_status" = 423
# TODO: Re-enable terraform force-unlock after capturing its request shape; see docs/terraform-force-unlock-investigation.md.
# terraform force-unlock -force "$lock_id"
curl --fail --silent --show-error --user "$TF_HTTP_USERNAME:$TF_HTTP_PASSWORD" --request UNLOCK --header 'Content-Type: application/json' --data "$lock_payload" "$TF_HTTP_ADDRESS" >/dev/null
curl --fail --silent --show-error --user "$TF_HTTP_USERNAME:$TF_HTTP_PASSWORD" --request LOCK --header 'Content-Type: application/json' --data '{"Created":"","ID":"post-force-lock","Info":"","Operation":"OperationTypeApply","Path":"","Version":"","Who":"integration-test"}' "$TF_HTTP_ADDRESS" >/dev/null
curl --fail --silent --show-error --user "$TF_HTTP_USERNAME:$TF_HTTP_PASSWORD" --request UNLOCK --header 'Content-Type: application/json' --data '{"Created":"","ID":"post-force-lock","Info":"","Operation":"","Path":"","Version":"","Who":""}' "$TF_HTTP_ADDRESS" >/dev/null
curl --fail --silent --show-error --user "$TF_HTTP_USERNAME:$TF_HTTP_PASSWORD" --request DELETE "$TF_HTTP_ADDRESS" >/dev/null

inspection=$(inspect_state)
echo "$inspection" | grep -q '"states":1'
echo "$inspection" | grep -Eq '"versions":[3-9][0-9]*'
echo "$inspection" | grep -q '"deleted":true'

stop_server
start_server
inspection=$(inspect_state)
echo "$inspection" | grep -q '"states":1'
echo "$inspection" | grep -q '"deleted":true'
