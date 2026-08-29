#!/usr/bin/env bash
# SPDX-License-Identifier: MPL-2.0

set -euo pipefail

if [[ $# -ne 6 ]]; then
	echo "usage: $0 <united-bin> <data-dir> <owner-email> <group-slug> <username> <password>" >&2
	exit 2
fi

united_bin=$1
data_dir=$2
owner_email=$3
group_slug=$4
username=$5
password=$6

"$united_bin" test-provision \
	--dir="$data_dir" \
	--owner-email="$owner_email" \
	--group-slug="$group_slug" \
	--username="$username" \
	--password="$password"
