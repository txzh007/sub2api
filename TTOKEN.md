# TToken

TToken is an independently versioned distribution based on Sub2API.

- TToken version: `0.4.0`
- Upstream baseline: `Sub2API 0.2.4` (`98d86915becae9fe9491a91ffc6defd5235c8d2b`)
- Product-facing name: `TToken`
- Upstream-compatible Go module and internal paths are intentionally retained
  to keep future Sub2API merges manageable.

## Version policy

`backend/cmd/server/TTOKEN_VERSION` is the TToken release version and is never
replaced by an upstream merge. `backend/cmd/server/VERSION` records the merged
Sub2API baseline.

Release tags use `ttoken-vX.Y.Z`; upstream-style `vX.Y.Z` tags remain outside
the TToken update channel.

Pushing a `ttoken-vX.Y.Z` tag runs the independent TToken release workflow. It
requires the tag version to match `backend/cmd/server/TTOKEN_VERSION`, runs the
full CI workflow, and publishes both:

- a Linux AMD64 image as `ghcr.io/txzh007/ttoken:X.Y.Z` and
  `ghcr.io/txzh007/ttoken:latest`;
- `ttoken_X.Y.Z_linux_amd64.tar.gz` plus `checksums.txt` for standalone binary
  installs and in-place online updates.

The upstream `backend/cmd/server/VERSION` file is not modified.

## Binary online updates

TToken checks only `ttoken-v*` releases from `txzh007/sub2api`; upstream
Sub2API `v*` releases cannot appear as TToken updates. A release binary verifies
the downloaded archive against `checksums.txt`, atomically replaces its own
executable, and relies on the `ttoken.service` restart policy to start the new
version. The previous executable remains as `sub2api.backup` for local rollback.

Container builds are intentionally marked as `container` and reject in-place
binary replacement. Replacing a binary inside an immutable image would be lost
when the container is recreated. Use the host to update container images, or
perform the one-time migration to the standalone binary service with
`deploy/install-ttoken.sh`; after that, the TToken version menu supports online
update and rollback.

The fallback catalog and Git-managed TToken pricing baseline are embedded in
the release binary. A standalone install therefore keeps domestic RMB, image,
and per-request video pricing without requiring a source checkout. Writable
administrator overrides remain in `data/model_pricing_overrides.json` and are
never replaced by a binary update.

## TToken 0.4.0

- Adds an independent `ttoken-v*` online-update channel backed by checksummed
  Linux AMD64 release archives.
- Adds a systemd deployment unit and installer with verified install, upgrade,
  local backup, and rollback support.
- Prevents container builds from replacing their own ephemeral executable and
  shows deployment-specific upgrade guidance in the admin UI.
- Embeds TToken's managed pricing and fallback catalog into standalone binaries.

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

## TToken 0.3.0

- Makes image-provider accounts a first-class account purpose backed by a
  stable image-generation group role rather than a mutable display name.
- Lets administrators create providers or copy supported existing API-key
  accounts while retaining only still-image model mappings.
- Uses one API-key bridge selection policy on Images and Responses: an empty
  value disables the bridge, a concrete model pins it, and null inherits the
  server default.
- Reports why configured image models are unavailable and keeps Grok video
  models outside the still-image bridge.
- Allows image providers to save an empty model selection safely.

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
