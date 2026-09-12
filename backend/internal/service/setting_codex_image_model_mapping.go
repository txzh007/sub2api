package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const DefaultCodexBuiltinImageModel = "gpt-image-2"

// CodexImageModelMappingSettings is the single site-wide alias used by the
// native Codex image tool. The target is a public model exposed by the image
// group; account model_mapping may still translate it for the provider.
type CodexImageModelMappingSettings struct {
	BuiltinModel string `json:"builtin_model"`
	TargetModel  string `json:"target_model"`
}

func DefaultCodexImageModelMappingSettings() *CodexImageModelMappingSettings {
	return &CodexImageModelMappingSettings{BuiltinModel: DefaultCodexBuiltinImageModel}
}

func (s *SettingService) defaultCodexImageModelMappingSettings() *CodexImageModelMappingSettings {
	settings := DefaultCodexImageModelMappingSettings()
	if s != nil && s.cfg != nil {
		settings.TargetModel = strings.TrimSpace(s.cfg.Gateway.CodexGeminiImageModel)
	}
	return settings
}

func normalizeCodexImageModelMappingSettings(settings *CodexImageModelMappingSettings) (*CodexImageModelMappingSettings, error) {
	if settings == nil {
		return nil, fmt.Errorf("settings cannot be nil")
	}
	result := &CodexImageModelMappingSettings{
		BuiltinModel: strings.TrimSpace(settings.BuiltinModel),
		TargetModel:  strings.TrimSpace(settings.TargetModel),
	}
	if result.BuiltinModel == "" {
		return nil, fmt.Errorf("builtin_model is required")
	}
	if strings.Contains(result.BuiltinModel, "*") || strings.Contains(result.TargetModel, "*") {
		return nil, fmt.Errorf("Codex image model mapping does not support wildcards")
	}
	if len(result.BuiltinModel) > 200 || len(result.TargetModel) > 200 {
		return nil, fmt.Errorf("Codex image model names must not exceed 200 characters")
	}
	if !IsImageProviderModel(result.BuiltinModel) {
		return nil, fmt.Errorf("builtin_model must be a still-image model")
	}
	return result, nil
}

func (s *SettingService) GetCodexImageModelMappingSettings(ctx context.Context) (*CodexImageModelMappingSettings, error) {
	if s == nil || s.settingRepo == nil {
		return s.defaultCodexImageModelMappingSettings(), nil
	}
	value, err := s.settingRepo.GetValue(ctx, SettingKeyCodexImageModelMapping)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return s.defaultCodexImageModelMappingSettings(), nil
		}
		return nil, fmt.Errorf("get Codex image model mapping: %w", err)
	}
	if strings.TrimSpace(value) == "" {
		return s.defaultCodexImageModelMappingSettings(), nil
	}
	var settings CodexImageModelMappingSettings
	if err := json.Unmarshal([]byte(value), &settings); err != nil {
		return nil, fmt.Errorf("decode Codex image model mapping: %w", err)
	}
	return normalizeCodexImageModelMappingSettings(&settings)
}

func (s *SettingService) SetCodexImageModelMappingSettings(ctx context.Context, settings *CodexImageModelMappingSettings) error {
	if s == nil || s.settingRepo == nil {
		return fmt.Errorf("setting repository is unavailable")
	}
	normalized, err := normalizeCodexImageModelMappingSettings(settings)
	if err != nil {
		return err
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return fmt.Errorf("encode Codex image model mapping: %w", err)
	}
	return s.settingRepo.Set(ctx, SettingKeyCodexImageModelMapping, string(data))
}

func (s *SettingService) ResolveCodexImageModel(ctx context.Context, model string) string {
	model = strings.TrimSpace(model)
	settings, err := s.GetCodexImageModelMappingSettings(ctx)
	if err != nil || settings.TargetModel == "" || model != settings.BuiltinModel {
		return model
	}
	return settings.TargetModel
}
