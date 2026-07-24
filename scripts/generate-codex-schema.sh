#!/usr/bin/env bash
set -euo pipefail

expected_version="codex-cli 0.145.0"
actual_version="$(codex --version)"

if [[ "$actual_version" != "$expected_version" ]]; then
  echo "Expected $expected_version, found $actual_version" >&2
  exit 1
fi

output_dir="$(mktemp -d "${TMPDIR:-/tmp}/linux-codex-schema.XXXXXX")"
trap 'rm -rf "$output_dir"' EXIT

codex app-server generate-json-schema --out "$output_dir"
cp "$output_dir/codex_app_server_protocol.v2.schemas.json" \
  internal/codex/schema/codex_app_server_protocol.v2.schemas.json
