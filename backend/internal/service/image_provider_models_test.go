package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsImageProviderModelExcludesTextAndVideoModels(t *testing.T) {
	tests := map[string]bool{
		"gpt-image-2":                   true,
		"dall-e-3":                      true,
		"models/gemini-3.1-flash-image": true,
		"grok-imagine":                  true,
		"xai/grok-imagine-image-2.0":    true,
		"gpt-5.6-sol":                   false,
		"gemini-3.1-pro-preview":        false,
		"grok-4.6":                      false,
		"grok-imagine-video-1.5":        false,
		"x-ai/grok-imagine-video":       false,
	}

	for model, expected := range tests {
		require.Equal(t, expected, IsImageProviderModel(model), model)
	}
}

func TestImageProviderModelMappingKeepsOnlyStillImageMappings(t *testing.T) {
	account := &Account{
		Platform: PlatformGrok,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"grok-4.6":               "grok-4.6",
				"draw":                   "grok-imagine-image-quality",
				"grok-imagine-image":     "vendor-custom-model",
				"grok-imagine-video-1.5": "grok-imagine-video-1.5",
				"gpt-*":                  "grok-imagine-image-quality",
			},
		},
	}

	require.Equal(t, map[string]string{
		"draw":               "grok-imagine-image-quality",
		"grok-imagine-image": "vendor-custom-model",
	}, ImageProviderModelMapping(account))
}

func TestImageProviderModelMappingUsesImageDefaultsWhenMappingIsEmpty(t *testing.T) {
	openAIModels := ImageProviderModelMapping(&Account{Platform: PlatformOpenAI})
	require.Contains(t, openAIModels, "gpt-image-2")
	require.NotContains(t, openAIModels, "gpt-5.6-sol")

	geminiModels := ImageProviderModelMapping(&Account{Platform: PlatformGemini})
	require.Contains(t, geminiModels, "gemini-3.1-flash-image")
	require.NotContains(t, geminiModels, "gemini-3.1-pro-preview")
}

func TestValidateImageProviderModelMappingAllowsEmptyAndRejectsTextOrVideo(t *testing.T) {
	require.NoError(t, ValidateImageProviderModelMapping(nil))
	require.NoError(t, ValidateImageProviderModelMapping(map[string]string{
		"draw": "grok-imagine-image-2.0",
	}))
	require.ErrorContains(t, ValidateImageProviderModelMapping(map[string]string{
		"grok-4.6": "grok-4.6",
	}), "not a still-image model")
	require.ErrorContains(t, ValidateImageProviderModelMapping(map[string]string{
		"grok-imagine-video-1.5": "grok-imagine-video-1.5",
	}), "not a still-image model")
	require.Error(t, ValidateImageProviderModelMapping(map[string]string{
		"gpt-*": "gpt-image-2",
	}))
}
