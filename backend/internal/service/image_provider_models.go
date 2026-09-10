package service

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

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
	}
	return result
}
