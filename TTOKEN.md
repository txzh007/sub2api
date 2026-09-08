# TToken

TToken is an independently versioned distribution based on Sub2API.

- TToken version: `0.1.0`
- Upstream baseline: `Sub2API v0.2.3`
- Product-facing name: `TToken`
- Upstream-compatible Go module and internal paths are intentionally retained
  to keep future Sub2API merges manageable.

## Version policy

`backend/cmd/server/TTOKEN_VERSION` is the TToken release version and is never
replaced by an upstream merge. `backend/cmd/server/VERSION` records the merged
Sub2API baseline.

Recommended release tags use `ttoken-vX.Y.Z`. Plain `vX.Y.Z` tags are also
accepted by the build tooling for compatibility.
