#!/usr/bin/env bash
# SPDX-License-Identifier: MPL-2.0

set -euo pipefail

if [[ $# -ne 4 ]]; then
	echo "usage: $0 <base-url> <group-slug> <username> <password>" >&2
	exit 2
fi

base_url=$1
group_slug=$2
username=$3
password=$4

user_payload=$(jq -n --arg identity 'user@example.com' --arg password 'foofoofoo' '{identity: $identity, password: $password}')
user_token=$(curl --fail-with-body --silent --show-error \
	--header 'Content-Type: application/json' \
	--data "$user_payload" \
	"$base_url/api/collections/users/auth-with-password" | jq -er '.token')
group_payload=$(jq -n \
	--arg email "$username@terraform.invalid" \
	--arg username "$username" \
	--arg slug "$group_slug" \
	--arg display_name "$group_slug" \
	--arg password "$password" \
	'{email: $email, username: $username, slug: $slug, displayName: $display_name, password: $password, passwordConfirm: $password}')
curl --fail-with-body --silent --show-error \
	--header "Authorization: Bearer $user_token" \
	--header 'Content-Type: application/json' \
	--data "$group_payload" \
	"$base_url/api/collections/groups/records" | jq -er '.id'
