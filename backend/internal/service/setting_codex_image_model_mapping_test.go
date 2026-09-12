//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestCodexImageModelMappingSettingsDefaultAndResolve(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo, &config.Config{})
	ctx := context.Background()

	settings, err := svc.GetCodexImageModelMappingSettings(ctx)
	require.NoError(t, err)
	require.Equal(t, DefaultCodexBuiltinImageModel, settings.BuiltinModel)
	require.Empty(t, settings.TargetModel)
	require.Equal(t, DefaultCodexBuiltinImageModel, svc.ResolveCodexImageModel(ctx, DefaultCodexBuiltinImageModel))

	require.NoError(t, svc.SetCodexImageModelMappingSettings(ctx, &CodexImageModelMappingSettings{
		BuiltinModel: " gpt-image-2 ",
		TargetModel:  " private-image-alias ",
	}))
	settings, err = svc.GetCodexImageModelMappingSettings(ctx)
	require.NoError(t, err)
	require.Equal(t, "gpt-image-2", settings.BuiltinModel)
	require.Equal(t, "private-image-alias", settings.TargetModel)
	require.Equal(t, "private-image-alias", svc.ResolveCodexImageModel(ctx, "gpt-image-2"))
	require.Equal(t, "gpt-image-1", svc.ResolveCodexImageModel(ctx, "gpt-image-1"))
}

func TestCodexImageModelMappingSettingsValidation(t *testing.T) {
	svc := NewSettingService(newMockSettingRepo(), &config.Config{})
	ctx := context.Background()

	for _, settings := range []*CodexImageModelMappingSettings{
		nil,
		{BuiltinModel: ""},
		{BuiltinModel: "gpt-5.4", TargetModel: "gemini-3.1-flash-image"},
		{BuiltinModel: "gpt-image-*", TargetModel: "gemini-3.1-flash-image"},
		{BuiltinModel: "gpt-image-2", TargetModel: "gemini-*-image"},
	} {
		require.Error(t, svc.SetCodexImageModelMappingSettings(ctx, settings))
	}

	require.NoError(t, svc.SetCodexImageModelMappingSettings(ctx, &CodexImageModelMappingSettings{
		BuiltinModel: "gpt-image-2",
		TargetModel:  "",
	}))
}

func TestCodexImageModelMappingMigratesAndCanDisableLegacyDefault(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo, &config.Config{Gateway: config.GatewayConfig{
		CodexGeminiImageModel: "gemini-3.1-flash-image",
	}})
	ctx := context.Background()

	settings, err := svc.GetCodexImageModelMappingSettings(ctx)
	require.NoError(t, err)
	require.Equal(t, "gemini-3.1-flash-image", settings.TargetModel)

	require.NoError(t, svc.SetCodexImageModelMappingSettings(ctx, &CodexImageModelMappingSettings{
		BuiltinModel: "gpt-image-2",
		TargetModel:  "",
	}))
	settings, err = svc.GetCodexImageModelMappingSettings(ctx)
	require.NoError(t, err)
	require.Empty(t, settings.TargetModel)
}
