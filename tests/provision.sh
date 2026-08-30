#!/usr/bin/env bash
# SPDX-License-Identifier: MPL-2.0

set -euo pipefail

if [[ $# -ne 6 ]]; then
	echo "usage: $0 <base-url> <admin-email> <admin-password> <owner-email> <group-slug> <username>" >&2
	exit 2
fi

base_url=$1
admin_email=$2
admin_password=$3
owner_email=$4
group_slug=$5
username=$6
password=${TF_HTTP_PASSWORD:?TF_HTTP_PASSWORD must be set}

authenticate() {
	local collection=$1
	local identity=$2
	local credential=$3
	local payload

	payload=$(jq -n --arg identity "$identity" --arg password "$credential" '{identity: $identity, password: $password}')
	curl --fail-with-body --silent --show-error \
		--header 'Content-Type: application/json' \
		--data "$payload" \
		"$base_url/api/collections/$collection/auth-with-password" | jq -er '.token'
}

admin_token=$(authenticate _superusers "$admin_email" "$admin_password")
owner_payload=$(jq -n \
	--arg email "$owner_email" \
	--arg password "$password" \
	'{email: $email, password: $password, passwordConfirm: $password}')
curl --fail-with-body --silent --show-error \
	--header "Authorization: Bearer $admin_token" \
	--header 'Content-Type: application/json' \
	--data "$owner_payload" \
	"$base_url/api/collections/users/records" >/dev/null

owner_token=$(authenticate users "$owner_email" "$password")
group_payload=$(jq -n \
	--arg email "$username@terraform.invalid" \
	--arg username "$username" \
	--arg slug "$group_slug" \
	--arg display_name "$group_slug" \
	--arg password "$password" \
	'{email: $email, username: $username, slug: $slug, displayName: $display_name, password: $password, passwordConfirm: $password}')
curl --fail-with-body --silent --show-error \
	--header "Authorization: Bearer $owner_token" \
	--header 'Content-Type: application/json' \
	--data "$group_payload" \
	"$base_url/api/collections/groups/records" | jq -er '.id'
