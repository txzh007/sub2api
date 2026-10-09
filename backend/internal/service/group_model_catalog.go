package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// GroupModelCatalogService shares the configuration catalog between the admin
// allowlist picker and the model plaza. Entries describe configured model names,
// not the current health or capacity of upstream accounts.
type GroupModelCatalogService struct {
	groupRepo   GroupRepository
	accountRepo AccountRepository
	channelRepo ChannelRepository
	routeRepo   CompositeModelRouteRepository
}

func NewGroupModelCatalogService(groups GroupRepository, accounts AccountRepository, channels ChannelRepository, routes CompositeModelRouteRepository) *GroupModelCatalogService {
	return &GroupModelCatalogService{groupRepo: groups, accountRepo: accounts, channelRepo: channels, routeRepo: routes}
}

func (s *GroupModelCatalogService) ListCandidates(ctx context.Context, groupID int64, platform string) ([]string, error) {
	g := &Group{ID: groupID, Platform: strings.TrimSpace(platform)}
	if groupID > 0 {
		stored, err := s.groupRepo.GetByIDLite(ctx, groupID)
		if err != nil {
			return nil, err
		}
		g = stored
		if platform = strings.TrimSpace(platform); platform != "" {
			copy := *stored
			copy.Platform = platform
			g = &copy
		}
	}
	if g.Platform == "" {
		g.Platform = PlatformAnthropic
	}
	var channels []Channel
	if groupID > 0 {
		var err error
		channels, err = s.channelRepo.ListAll(ctx)
		if err != nil {
			return nil, fmt.Errorf("list model catalog channels: %w", err)
		}
	}
	models, err := s.forGroup(ctx, g, channels)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(models))
	seen := make(map[string]bool)
	for _, model := range models {
		key := strings.ToLower(model.Name)
		if !seen[key] {
			seen[key] = true
			names = append(names, model.Name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func (s *GroupModelCatalogService) forGroup(ctx context.Context, g *Group, channels []Channel) ([]SupportedModel, error) {
	models := make([]SupportedModel, 0)
	seen := make(map[string]int)
	add := func(name, platform string, pricing *ChannelModelPricing) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if platform == "" || platform == PlatformComposite {
			platform = g.Platform
			if platform == PlatformComposite {
				if detected, ok := DetectModelPlatform(name); ok {
					platform = detected
				}
			}
		}
		if g.Platform != PlatformComposite && platform != g.Platform {
			return
		}
		key := platform + "\x00" + strings.ToLower(name)
		if at, ok := seen[key]; ok {
			if models[at].Pricing == nil && pricing != nil {
				models[at].Pricing = pricing
			}
			return
		}
		seen[key] = len(models)
		models = append(models, SupportedModel{Name: name, Platform: platform, Pricing: pricing})
	}
	// Keep channel ordering/pricing consistent with the legacy plaza.
	for i := range channels {
		ch := &channels[i]
		if ch.Status != StatusActive {
			continue
		}
		for _, id := range ch.GroupIDs {
			if id != g.ID {
				continue
			}
			for _, m := range ch.SupportedModels() {
				add(m.Name, m.Platform, m.Pricing)
			}
			break
		}
	}
	if g.ID > 0 {
		accounts, err := s.accountRepo.ListByGroup(ctx, g.ID)
		if err != nil {
			return nil, fmt.Errorf("list model catalog accounts: %w", err)
		}
		for i := range accounts {
			account := &accounts[i]
			mapping := account.GetModelMapping()
			if g.SystemRole == GroupSystemRoleImageGeneration {
				mapping = ImageProviderRoutingModelMapping(account)
			}
			for model := range mapping {
				if !strings.Contains(model, "*") {
					add(model, account.Platform, nil)
				}
			}
		}
		if g.Platform == PlatformComposite && s.routeRepo != nil {
			routes, err := s.routeRepo.ListByGroup(ctx, g.ID, false)
			if err != nil {
				return nil, fmt.Errorf("list model catalog routes: %w", err)
			}
			for _, route := range routes {
				if route.Enabled && normalizeCompositeRouteMatchType(route.MatchType) == CompositeRouteMatchExact {
					add(route.PublicModel, route.TargetPlatform, nil)
				}
			}
		}
	}
	for i := range g.ModelPricing {
		pricing := &g.ModelPricing[i]
		for _, name := range pricing.Models {
			if !strings.Contains(name, "*") {
				add(name, pricing.Platform, pricing)
			}
		}
	}
	// Defaults and saved custom names only need a fallback platform when no
	// account, channel or explicit route already identifies their provider.
	addFallback := func(name string) {
		for _, existing := range models {
			if strings.EqualFold(existing.Name, name) {
				return
			}
		}
		add(name, g.Platform, nil)
	}
	for _, name := range groupCatalogDefaultModelIDs(g.Platform) {
		if g.SystemRole != GroupSystemRoleImageGeneration || IsImageProviderModel(name) {
			addFallback(name)
		}
	}
	for _, name := range g.ModelAllowlist.Models {
		addFallback(name)
	}
	sort.SliceStable(models, func(i, j int) bool {
		if models[i].Name != models[j].Name {
			return models[i].Name < models[j].Name
		}
		return models[i].Platform < models[j].Platform
	})
	return models, nil
}

func groupCatalogDefaultModelIDs(platform string) []string {
	switch platform {
	case PlatformAnthropic, PlatformOpenAI, PlatformGemini, PlatformAntigravity,
		PlatformGrok, PlatformOpenCodeGo, PlatformTypeSafe, PlatformComposite:
		return defaultModelsListCandidateIDs(platform)
	default:
		// Providers without a built-in catalog use their configured account,
		// channel and pricing names, instead of the legacy Claude fallback.
		return nil
	}
}
