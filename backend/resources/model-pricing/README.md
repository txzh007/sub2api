# Model Pricing Data

This directory contains a local copy of the mirrored model pricing data as a fallback mechanism.

It also contains `model_price_sources.json`, TToken's auditable provenance manifest for
domestic-model prices. The manifest is intentionally separate from the runtime price table:

- `model_prices_and_context_window.json`, `ttoken_model_pricing_overrides.json`, and the
  writable local override file determine billing in that priority order.
- `model_price_sources.json` records where a price came from, when it was checked, its
  currency/unit, and whether it is official, an operator policy, or provisional.
- `models.dev` is a discovery and cross-check source only. Its prices are USD and must not
  be copied into TToken's CNY ledger for domestic models.

## Source
The original file is maintained by the LiteLLM project and mirrored into the `price-mirror` branch of this repository via GitHub Actions:
- Mirror branch (configurable via `PRICE_MIRROR_REPO`): https://raw.githubusercontent.com/<your-repo>/price-mirror/model_prices_and_context_window.json
- Upstream source: https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json

## Purpose
This local copy serves as a fallback when the remote file cannot be downloaded due to:
- Network restrictions
- Firewall rules
- DNS resolution issues
- GitHub being blocked in certain regions
- Docker container network limitations

## Update Process
The pricingService will:
1. First attempt to download the latest version from GitHub
2. If download fails, use this local copy as fallback
3. Log a warning when using the fallback file

## Manual Update
To manually update this file with the latest pricing data (if automation is unavailable):
```bash
curl -s https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json -o model_prices_and_context_window.json
```

## File Format
The file contains JSON data with model pricing information including:
- Model names and identifiers
- Input/output token costs
- Context window sizes
- Model capabilities

The provenance manifest uses human-readable prices per one million tokens. Runtime files use
per-token numbers, so a manifest value of `6` corresponds to `0.000006` in the runtime table.
Entries with `inherits` are aliases and inherit the price, provider, currency and unit from
their target unless explicitly overridden.

`ttoken_model_pricing_overrides.json` is the Git-managed deployable baseline. It contains
the exact per-token/per-image/per-video values used by TToken. The administrator-managed
`data/model_pricing_overrides.json` remains a separate, writable highest-priority layer, so
an image upgrade can refresh the baseline without erasing local pricing decisions.

Last updated: 2025-08-10
