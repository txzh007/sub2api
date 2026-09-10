package service

import (
	"context"
	"sort"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const ImageBridgeGroupName = "生图"

const (
	ImageBridgeUnavailableNoProvider           = "no_provider"
	ImageBridgeUnavailableAccountInactive      = "account_inactive"
	ImageBridgeUnavailableAccountUnschedulable = "account_unschedulable"
	ImageBridgeUnavailableWrongPurpose         = "wrong_account_purpose"
	ImageBridgeUnavailableNotAllowed           = "not_in_group_allowlist"
	ImageBridgeUnavailableTemporary            = "temporarily_unavailable"
)

func KeySupportsImageBridge(key *APIKey) bool {
	return key != nil && key.Group != nil && key.Group.Platform == PlatformOpenAI
}

func validateImageBridgePrimaryGroup(group *Group) error {
	if group == nil || group.Platform != PlatformOpenAI {
		return infraerrors.BadRequest("IMAGE_BRIDGE_REQUIRES_OPENAI_GROUP", "仅 OpenAI 分组可以配置生图桥接")
	}
	return nil
}

type ImageBridgeModels struct {
	GroupID          int64                          `json:"group_id,omitempty"`
	GroupName        string                         `json:"group_name"`
	DefaultModel     string                         `json:"default_model,omitempty"`
	Models           []string                       `json:"models"`
	Availability     []ImageBridgeModelAvailability `json:"availability"`
	GroupUnavailable bool                           `json:"group_unavailable,omitempty"`
}

type ImageBridgeModelAvailability struct {
	Model     string `json:"model"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// EffectiveImageBridgeModel applies the same persisted selection semantics to
// every bridge endpoint: empty disables, a concrete value pins the model, and
// nil inherits the server default.
func (s *APIKeyService) EffectiveImageBridgeModel(key *APIKey) (string, bool) {
	if key == nil {
		return "", false
	}
	if key.ImageBridgeModel != nil {
		model := strings.TrimSpace(*key.ImageBridgeModel)
		return model, model != ""
	}
	if s == nil || s.cfg == nil {
		return "", false
	}
	model := strings.TrimSpace(s.cfg.Gateway.CodexGeminiImageModel)
	return model, model != ""
}

// The image group remains the source of model availability, access and pricing.
// Do not copy its account credentials or model list into each API key.
func (s *APIKeyService) imageBridgeGroup(ctx context.Context, userID int64) (*Group, error) {
	groups, err := s.GetAvailableGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range groups {
		if groups[i].IsImageGenerationGroup() && GroupAllowsImageGeneration(&groups[i]) {
			return s.groupRepo.GetByID(ctx, groups[i].ID)
		}
	}
	return nil, nil
}

func (s *APIKeyService) imageBridgeModelAvailabilityForGroup(ctx context.Context, group *Group) ([]ImageBridgeModelAvailability, error) {
	availability := []ImageBridgeModelAvailability{}
	if group == nil || s.imageBridgeAccounts == nil {
		return availability, nil
	}
	allAccounts, err := s.imageBridgeAccounts.ListByGroup(ctx, group.ID)
	if err != nil {
		return nil, err
	}
	schedulableAccounts, err := s.imageBridgeAccounts.ListSchedulableByGroupID(ctx, group.ID)
	if err != nil {
		return nil, err
	}
	schedulableIDs := make(map[int64]struct{}, len(schedulableAccounts))
	for i := range schedulableAccounts {
		schedulableIDs[schedulableAccounts[i].ID] = struct{}{}
	}

	providersByModel := map[string][]Account{}
	for _, account := range allAccounts {
		if account.Platform != PlatformGemini && account.Platform != PlatformOpenAI && account.Platform != PlatformGrok {
			continue
		}
		if group.Platform != PlatformComposite && group.Platform != account.Platform {
			continue
		}
		for model, target := range account.GetModelMapping() {
			if strings.Contains(model, "*") || strings.TrimSpace(model) == "" {
				continue
			}
			if !IsImageProviderModel(model) && !IsImageProviderModel(target) {
				continue
			}
			providersByModel[model] = append(providersByModel[model], account)
		}
	}

	candidates := map[string]struct{}{}
	for model := range providersByModel {
		candidates[model] = struct{}{}
	}
	if group.ModelAllowlistEnabled() {
		for _, model := range group.ModelAllowlist.Models {
			if !strings.Contains(model, "*") && IsImageProviderModel(model) {
				candidates[model] = struct{}{}
			}
		}
	}

	models := make([]string, 0, len(candidates))
	for model := range candidates {
		models = append(models, model)
	}
	sort.Strings(models)
	for _, model := range models {
		status := ImageBridgeModelAvailability{Model: model}
		providers := providersByModel[model]
		if len(providers) == 0 {
			status.Reason = ImageBridgeUnavailableNoProvider
			availability = append(availability, status)
			continue
		}
		if group.ModelAllowlistEnabled() && !group.ModelAllowlist.Allows(model) {
			status.Reason = ImageBridgeUnavailableNotAllowed
			availability = append(availability, status)
			continue
		}
		reason := ImageBridgeUnavailableTemporary
		for i := range providers {
			provider := &providers[i]
			if !provider.IsImageProvider() {
				reason = ImageBridgeUnavailableWrongPurpose
				continue
			}
			if provider.Status != StatusActive {
				reason = ImageBridgeUnavailableAccountInactive
				continue
			}
			if !provider.Schedulable {
				reason = ImageBridgeUnavailableAccountUnschedulable
				continue
			}
			if _, ok := schedulableIDs[provider.ID]; ok {
				status.Available = true
				status.Reason = ""
				break
			}
		}
		if !status.Available {
			status.Reason = reason
		}
		availability = append(availability, status)
	}
	return availability, nil
}

func (s *APIKeyService) imageBridgeModelsForGroup(ctx context.Context, group *Group) ([]string, error) {
	availability, err := s.imageBridgeModelAvailabilityForGroup(ctx, group)
	if err != nil {
		return nil, err
	}
	models := make([]string, 0, len(availability))
	for _, item := range availability {
		if item.Available {
			models = append(models, item.Model)
		}
	}
	return models, nil
}

func isBridgeImageModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(model, "gpt-image-") || strings.HasPrefix(model, "dall-e-") ||
		(strings.HasPrefix(model, "gemini-") && strings.Contains(model, "-image")) ||
		(strings.HasPrefix(model, "grok-") && strings.Contains(model, "image"))
}

func RewriteImageBridgeModel(body []byte, contentType, model string) ([]byte, string, error) {
	return rewriteOpenAIImagesModel(body, contentType, model)
}

func (s *APIKeyService) ImageBridgeAPIKey(ctx context.Context, key *APIKey, model string) (*APIKey, *UserSubscription, error) {
	if key == nil {
		return nil, nil, validateImageBridgePrimaryGroup(nil)
	}
	if err := validateImageBridgePrimaryGroup(key.Group); err != nil {
		return nil, nil, err
	}
	group, sub, err := s.ResolveImageBridge(ctx, key.UserID, model)
	if err != nil {
		return nil, nil, err
	}
	imageKey := *key
	imageKey.GroupID = &group.ID
	imageKey.Group = group
	if key.User != nil {
		user := *key.User
		user.UserGroupRPMOverride = nil
		if s.userGroupRateRepo != nil {
			user.UserGroupRPMOverride, err = s.userGroupRateRepo.GetRPMOverrideByUserAndGroup(ctx, user.ID, group.ID)
			if err != nil {
				return nil, nil, err
			}
		}
		imageKey.User = &user
	}
	return &imageKey, sub, nil
}

func (s *APIKeyService) GetImageBridgeModels(ctx context.Context, userID int64) (*ImageBridgeModels, error) {
	group, err := s.imageBridgeGroup(ctx, userID)
	if err != nil {
		return nil, err
	}
	availability, err := s.imageBridgeModelAvailabilityForGroup(ctx, group)
	if err != nil {
		return nil, err
	}
	result := &ImageBridgeModels{GroupName: ImageBridgeGroupName, Availability: availability}
	if s.cfg != nil {
		result.DefaultModel = strings.TrimSpace(s.cfg.Gateway.CodexGeminiImageModel)
	}
	if group != nil {
		result.GroupID = group.ID
		result.GroupName = group.Name
		for _, item := range availability {
			if item.Available {
				result.Models = append(result.Models, item.Model)
			}
		}
	} else {
		result.GroupUnavailable = true
	}
	return result, nil
}

// ResolveImageBridge rechecks the image group's current access and subscription
// so editing/deleting/disabling a group cannot leave a stale cross-group grant.
func (s *APIKeyService) ResolveImageBridge(ctx context.Context, userID int64, model string) (*Group, *UserSubscription, error) {
	group, err := s.imageBridgeGroup(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	if group == nil || !group.IsActive() || !GroupAllowsImageGeneration(group) {
		return nil, nil, infraerrors.Forbidden("IMAGE_BRIDGE_GROUP_UNAVAILABLE", "生图分组未启用、未开放图片生成，或当前用户无权使用")
	}
	models, err := s.imageBridgeModelsForGroup(ctx, group)
	if err != nil {
		return nil, nil, err
	}
	found := false
	for _, candidate := range models {
		if candidate == model {
			found = true
			break
		}
	}
	if !found {
		return nil, nil, infraerrors.BadRequest("IMAGE_BRIDGE_MODEL_UNAVAILABLE", "所选模型不在生图分组的可用模型列表中")
	}
	var sub *UserSubscription
	if group.IsSubscriptionType() {
		sub, err = s.userSubRepo.GetActiveByUserIDAndGroupID(ctx, userID, group.ID)
		if err != nil {
			return nil, nil, ErrGroupNotAllowed
		}
	}
	return group, sub, nil
}
