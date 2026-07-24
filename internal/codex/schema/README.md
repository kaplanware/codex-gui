# Codex app-server schema

The checked-in schema bundle is generated from `codex-cli 0.145.0` without
experimental APIs. Run `scripts/generate-codex-schema.sh` with that CLI version
to refresh it. Runtime protocol payloads stay behind the Go adapter rather than
being exposed directly to the Vue frontend.
