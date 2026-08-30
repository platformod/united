#!/usr/bin/env bash
# SPDX-License-Identifier: MPL-2.0

set -euo pipefail

united_bin=${UNITED_BIN:?UNITED_BIN must be set}
group_slug=${GROUP_SLUG:?GROUP_SLUG must be set}
state_name=${STATE_NAME:?STATE_NAME must be set}
TF_HTTP_USERNAME="terraform-$RANDOM-$RANDOM"
TF_HTTP_PASSWORD=$(openssl rand -base64 24)
ADMIN_EMAIL="admin-$RANDOM-$RANDOM@example.test"
ADMIN_PASSWORD=$(openssl rand -base64 24)
OWNER_EMAIL="owner-$RANDOM-$RANDOM@example.test"
export TF_HTTP_USERNAME TF_HTTP_PASSWORD ADMIN_EMAIL ADMIN_PASSWORD

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

authenticate_superuser() {
	local payload

	payload=$(jq -n --arg identity "$ADMIN_EMAIL" --arg password "$ADMIN_PASSWORD" '{identity: $identity, password: $password}')
	curl --fail-with-body --silent --show-error \
		--header 'Content-Type: application/json' \
		--data "$payload" \
		"$api_url/api/collections/_superusers/auth-with-password" | jq -er '.token'
}

inspect_state() {
	local admin_token state_metadata state_id version_metadata states versions deleted

	admin_token=$(authenticate_superuser)
	state_metadata=$(curl --fail-with-body --silent --show-error --get \
		--header "Authorization: Bearer $admin_token" \
		--data-urlencode "filter=group = \"$group_id\" && name = \"$state_name\"" \
		--data-urlencode 'fields=id,deletedAt' \
		"$api_url/api/collections/states/records")
	states=$(jq -er '.totalItems' <<<"$state_metadata")
	state_id=$(jq -er '.items[0].id // empty' <<<"$state_metadata")
	deleted=$(jq -er '(.items[0].deletedAt // "") != ""' <<<"$state_metadata")
	version_metadata=$(curl --fail-with-body --silent --show-error --get \
		--header "Authorization: Bearer $admin_token" \
		--data-urlencode "filter=state = \"$state_id\"" \
		--data-urlencode 'fields=id,state' \
		"$api_url/api/collections/statefiles/records")
	versions=$(jq -er '.totalItems' <<<"$version_metadata")

	jq -cn --argjson states "$states" --argjson versions "$versions" --argjson deleted "$deleted" \
		'{states: $states, versions: $versions, deleted: $deleted}'
}

export UNITED_STATE_MASTER_KEY
UNITED_STATE_MASTER_KEY=$(openssl rand -base64 32)
if [[ ! -e "$data_dir/data.db" ]]; then
	UNITED_STATE_MASTER_KEY="$UNITED_STATE_MASTER_KEY" "$united_bin" superuser upsert "$ADMIN_EMAIL" "$ADMIN_PASSWORD" --dir="$data_dir"
fi
start_server
api_url="http://127.0.0.1:$port"
group_id=$("$(dirname "$0")/provision.sh" "$api_url" "$ADMIN_EMAIL" "$ADMIN_PASSWORD" "$OWNER_EMAIL" "$group_slug" "$TF_HTTP_USERNAME")
export TF_HTTP_ADDRESS="$api_url/state/$group_slug/$state_name"
export TF_HTTP_LOCK_ADDRESS="$TF_HTTP_ADDRESS"
export TF_HTTP_UNLOCK_ADDRESS="$TF_HTTP_ADDRESS"
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
api_url="http://127.0.0.1:$port"
inspection=$(inspect_state)
echo "$inspection" | grep -q '"states":1'
echo "$inspection" | grep -q '"deleted":true'
