package service

import (
	"fmt"
	"sort"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

// SupportsOAuthImageProvider reports credential/platform eligibility. Opt-in is
// stored as membership of the image generation group, sharing the same account
// and token refresh lifecycle with its ordinary groups.
func (a *Account) SupportsOAuthImageProvider() bool {
	if a == nil || a.Type != AccountTypeOAuth || a.IsShadow() {
		return false
	}
	switch a.Platform {
	case PlatformOpenAI, PlatformGemini, PlatformGrok:
		return true
	default:
		return false
	}
}

// ValidateImageProviderModelMapping prevents dedicated image providers from
// accidentally exposing text or video models. An empty mapping is valid and
// leaves the provider configured but unavailable until models are selected.
func ValidateImageProviderModelMapping(mapping map[string]string) error {
	for requestedModel, upstreamModel := range mapping {
		if strings.Contains(requestedModel, "*") || strings.TrimSpace(requestedModel) == "" {
			return infraerrors.BadRequest("IMAGE_PROVIDER_MODEL_INVALID", "image provider model aliases must be explicit and non-empty")
		}
		if !IsImageProviderModel(requestedModel) && !IsImageProviderModel(upstreamModel) {
			return infraerrors.BadRequest(
				"IMAGE_PROVIDER_MODEL_INVALID",
				fmt.Sprintf("%s is not a still-image model", requestedModel),
			)
		}
	}
	return nil
}

// IsImageProviderModel reports whether model is a still-image generation model.
// Video models are intentionally excluded from the dedicated image-provider list.
func IsImageProviderModel(model string) bool {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if slash := strings.LastIndexByte(normalized, '/'); slash >= 0 {
		normalized = strings.TrimSpace(normalized[slash+1:])
	}
	return isOpenAIImageGenerationModel(normalized) || isBridgeImageModel(normalized)
}

// ImageProviderModelMapping returns a new mapping containing only still-image
// models. Accounts without an explicit mapping use the curated defaults for
// their provider so a copied image provider is immediately configurable.
func ImageProviderModelMapping(account *Account) map[string]string {
	if account == nil {
		return map[string]string{}
	}

	mapping := account.GetModelMapping()
	if len(mapping) == 0 {
		mapping = defaultImageProviderModelMapping(account.Platform)
	}

	filtered := make(map[string]string)
	for requestedModel, upstreamModel := range mapping {
		if strings.Contains(requestedModel, "*") || strings.TrimSpace(requestedModel) == "" {
			continue
		}
		if !IsImageProviderModel(requestedModel) && !IsImageProviderModel(upstreamModel) {
			continue
		}
		filtered[requestedModel] = upstreamModel
	}
	return filtered
}

func defaultImageProviderModelMapping(platform string) map[string]string {
	result := make(map[string]string)
	switch platform {
	case PlatformOpenAI:
		for _, model := range openai.DefaultModels {
			if IsImageProviderModel(model.ID) {
				result[model.ID] = model.ID
			}
		}
	case PlatformGemini:
		for _, model := range geminicli.DefaultModels {
			if IsImageProviderModel(model.ID) {
				result[model.ID] = model.ID
			}
		}
	case PlatformGrok:
		for model, target := range xai.DefaultModelMapping() {
			if IsImageProviderModel(model) || IsImageProviderModel(target) {
				result[model] = target
			}
		}
	}
	return result
}

// ImageProviderRoutingModelMapping preserves the explicit model selection of
// dedicated providers. Linked OAuth accounts also expose supported defaults,
// including defaults matched by their model whitelist's wildcard rules.
func ImageProviderRoutingModelMapping(account *Account) map[string]string {
	result := make(map[string]string)
	if account == nil {
		return result
	}
	for model, target := range account.GetModelMapping() {
		if strings.TrimSpace(model) != "" && !strings.Contains(model, "*") &&
			(IsImageProviderModel(model) || IsImageProviderModel(target)) {
			result[model] = target
		}
	}
	if account.SupportsOAuthImageProvider() {
		for model := range defaultImageProviderModelMapping(account.Platform) {
			if target := account.GetMappedModel(model); account.IsModelSupported(model) && IsImageProviderModel(target) {
				result[model] = target
			}
		}
	}
	return result
}

func ImageProviderModelNames(account *Account) []string {
	mapping := ImageProviderRoutingModelMapping(account)
	models := make([]string, 0, len(mapping))
	for model := range mapping {
		models = append(models, model)
	}
	sort.Strings(models)
	return models
}
