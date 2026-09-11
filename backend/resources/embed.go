// Package resources exposes the version-controlled pricing baseline to
// standalone release binaries. External files still take precedence, but a
// binary installed without the repository checkout must retain the same
// fallback catalog and TToken-managed overrides as the container image.
package resources

import _ "embed"

var (
	//go:embed model-pricing/model_prices_and_context_window.json
	fallbackPricing []byte

	//go:embed model-pricing/ttoken_model_pricing_overrides.json
	managedPricingOverrides []byte
)

func FallbackPricing() []byte {
	return append([]byte(nil), fallbackPricing...)
}

func ManagedPricingOverrides() []byte {
	return append([]byte(nil), managedPricingOverrides...)
}
