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
full CI workflow, and publishes a Linux AMD64 image as
`ghcr.io/txzh007/ttoken:X.Y.Z` and `ghcr.io/txzh007/ttoken:latest`. The upstream
`backend/cmd/server/VERSION` file is not modified.

## Blue-green Docker cutover

The `deploy/docker-compose.ttoken-green.yml` override adds a `ttoken-green`
service. It inherits the existing `sub2api` service's environment, PostgreSQL,
Redis, network, and `/app/data` volume, while publishing the application on
`127.0.0.1:18080` by default. The application listens on port `8080` inside the
candidate container even when the existing deployment uses another port.
Set a persistent 64-character hexadecimal `TOTP_ENCRYPTION_KEY` in `deploy/.env`
before starting the candidate; the override requires it so encrypted settings do
not become unreadable after a restart.

Start and verify the candidate without replacing the existing `sub2api`
container:

```bash
cd deploy
docker compose -f docker-compose.yml -f docker-compose.ttoken-green.yml pull ttoken-green
docker compose -f docker-compose.yml -f docker-compose.ttoken-green.yml \
  up -d --no-deps ttoken-green
curl -fsS http://127.0.0.1:18080/health
docker compose -f docker-compose.yml -f docker-compose.ttoken-green.yml \
  exec ttoken-green /app/sub2api --version
```

After verification, point the Nginx upstream at `127.0.0.1:18080`, reload
Nginx, and keep the old application container available for a short observation
window. Avoid administrator configuration writes while both application
versions are running. Database migrations are forward-only, so a complete
rollback requires the pre-upgrade PostgreSQL and `/app/data` backups.

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
