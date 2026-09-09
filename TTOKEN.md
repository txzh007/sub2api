# TToken

TToken is an independently versioned distribution based on Sub2API.

- TToken version: `0.2.0`
- Upstream baseline: `Sub2API 0.2.4` (`98d86915becae9fe9491a91ffc6defd5235c8d2b`)
- Product-facing name: `TToken`
- Upstream-compatible Go module and internal paths are intentionally retained
  to keep future Sub2API merges manageable.

## Version policy

`backend/cmd/server/TTOKEN_VERSION` is the TToken release version and is never
replaced by an upstream merge. `backend/cmd/server/VERSION` records the merged
Sub2API baseline.

Recommended release tags use `ttoken-vX.Y.Z`. Plain `vX.Y.Z` tags are also
accepted by the build tooling for compatibility.

Pushing a `ttoken-vX.Y.Z` tag runs the independent TToken release workflow. It
requires the tag version to match `backend/cmd/server/TTOKEN_VERSION`, runs the
full CI workflow, and publishes multi-architecture images as
`ghcr.io/txzh007/ttoken:X.Y.Z` and `ghcr.io/txzh007/ttoken:latest`. The upstream
`backend/cmd/server/VERSION` file is not modified.

## TToken 0.2.0

- Adds a global model pricing catalog scoped to models present in enabled groups.
- Keeps international model families in USD and domestic providers in a direct
  RMB numeric ledger without foreign-exchange conversion.
- Adds Git-managed pricing and provenance JSON files with a writable local
  administrator override layer.
- Applies the TToken DeepSeek weekday peak/off-peak policy in Asia/Shanghai.
- Adds image pricing coverage and per-request video pricing.
- Integrates the Sub2API 0.2.4 upstream snapshot, including MiniMax platform,
  GPT Image 2.5, Grok media eligibility, and gateway stability fixes.
