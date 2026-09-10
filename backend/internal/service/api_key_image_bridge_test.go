//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type imageBridgeGroupRepoStub struct {
	GroupRepository
	group *Group
}

func (r *imageBridgeGroupRepoStub) ListActive(context.Context) ([]Group, error) {
	if !r.group.IsActive() {
		return nil, nil
	}
	return []Group{*r.group}, nil
}
func (r *imageBridgeGroupRepoStub) GetByID(context.Context, int64) (*Group, error) {
	return r.group, nil
}

type imageBridgeUserRepoStub struct {
	UserRepository
	user *User
}

func (r *imageBridgeUserRepoStub) GetByID(context.Context, int64) (*User, error) { return r.user, nil }

type imageBridgeSubRepoStub struct {
	UserSubscriptionRepository
	subscriptions []UserSubscription
}

func (r *imageBridgeSubRepoStub) ListActiveByUserID(context.Context, int64) ([]UserSubscription, error) {
	return r.subscriptions, nil
}
func (r *imageBridgeSubRepoStub) GetActiveByUserIDAndGroupID(context.Context, int64, int64) (*UserSubscription, error) {
	if len(r.subscriptions) == 0 {
		return nil, ErrGroupNotAllowed
	}
	return &r.subscriptions[0], nil
}

type imageBridgeAccountRepoStub struct {
	AccountRepository
	accounts []Account
	groupID  int64
}

func (r *imageBridgeAccountRepoStub) ListSchedulableByGroupID(_ context.Context, id int64) ([]Account, error) {
	r.groupID = id
	return r.accounts, nil
}

func (r *imageBridgeAccountRepoStub) ListByGroup(_ context.Context, id int64) ([]Account, error) {
	r.groupID = id
	return r.accounts, nil
}

func imageBridgeServiceFixture() (*APIKeyService, *Group, *User, *imageBridgeAccountRepoStub) {
	group := &Group{ID: 24, Name: ImageBridgeGroupName, SystemRole: GroupSystemRoleImageGeneration, Platform: PlatformComposite, Status: StatusActive, AllowImageGeneration: true, Hydrated: true}
	user := &User{ID: 1, Status: StatusActive, Balance: 100}
	accounts := &imageBridgeAccountRepoStub{accounts: []Account{{ID: 9, Platform: PlatformGemini, Purpose: AccountPurposeImageProvider, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"model_mapping": map[string]any{"gemini-3.1-flash-image": "gemini-3.1-flash-image", "gemini-3-pro-image": "gemini-3-pro-image", "gpt-5.5": "gpt-5.5", "*": "gemini-3-pro-image"}}}}}
	svc := &APIKeyService{groupRepo: &imageBridgeGroupRepoStub{group: group}, userRepo: &imageBridgeUserRepoStub{user: user}, userSubRepo: &imageBridgeSubRepoStub{}, imageBridgeAccounts: accounts}
	return svc, group, user, accounts
}

func TestAPIKeyImageBridgeModelsUseImageGroupAndCurrentPermissions(t *testing.T) {
	svc, group, user, accounts := imageBridgeServiceFixture()
	ctx := context.Background()
	models, err := svc.GetImageBridgeModels(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, int64(24), accounts.groupID)
	require.Equal(t, []string{"gemini-3-pro-image", "gemini-3.1-flash-image"}, models.Models)
	for _, mutate := range []func(){
		func() { group.Status = "inactive" },
		func() { group.Status = StatusActive; group.AllowImageGeneration = false },
		func() {
			group.AllowImageGeneration = true
			group.Name = "改名后的生图池"
			group.SystemRole = GroupSystemRoleNone
		},
		func() { group.SystemRole = GroupSystemRoleImageGeneration; group.IsExclusive = true },
	} {
		mutate()
		models, err = svc.GetImageBridgeModels(ctx, user.ID)
		require.NoError(t, err)
		require.Empty(t, models.Models)
		_, _, err = svc.ResolveImageBridge(ctx, user.ID, "gemini-3.1-flash-image")
		require.Error(t, err)
	}
	user.AllowedGroups = []int64{24}
	_, _, err = svc.ResolveImageBridge(ctx, user.ID, "gemini-3.1-flash-image")
	require.NoError(t, err)
	_, _, err = svc.ResolveImageBridge(ctx, user.ID, "gemini-not-in-group-image")
	require.Error(t, err)
}

func TestAPIKeyImageBridgeGroupRenameDoesNotBreakLookup(t *testing.T) {
	svc, group, user, _ := imageBridgeServiceFixture()
	group.Name = "Images Pool"
	models, err := svc.GetImageBridgeModels(context.Background(), user.ID)
	require.NoError(t, err)
	require.Equal(t, "Images Pool", models.GroupName)
	require.Equal(t, []string{"gemini-3-pro-image", "gemini-3.1-flash-image"}, models.Models)
}

func TestAPIKeyImageBridgeEffectiveModelUsesSharedTriStateSemantics(t *testing.T) {
	svc := &APIKeyService{cfg: &config.Config{Gateway: config.GatewayConfig{CodexGeminiImageModel: " gemini-3.1-flash-image "}}}

	model, enabled := svc.EffectiveImageBridgeModel(&APIKey{})
	require.Equal(t, "gemini-3.1-flash-image", model)
	require.True(t, enabled)

	disabled := ""
	model, enabled = svc.EffectiveImageBridgeModel(&APIKey{ImageBridgeModel: &disabled})
	require.Empty(t, model)
	require.False(t, enabled)

	pinned := " grok-imagine-image-2.0 "
	model, enabled = svc.EffectiveImageBridgeModel(&APIKey{ImageBridgeModel: &pinned})
	require.Equal(t, "grok-imagine-image-2.0", model)
	require.True(t, enabled)
}

func TestAPIKeyImageBridgeAvailabilityExplainsPausedProvider(t *testing.T) {
	svc, group, user, accounts := imageBridgeServiceFixture()
	group.ModelAllowlist = GroupModelAllowlist{Enabled: true, Models: []string{"gemini-3.1-flash-image", "grok-imagine-image-2.0"}}
	accounts.accounts[0].Schedulable = false
	models, err := svc.GetImageBridgeModels(context.Background(), user.ID)
	require.NoError(t, err)
	require.Empty(t, models.Models)
	require.Equal(t, []ImageBridgeModelAvailability{
		{Model: "gemini-3-pro-image", Reason: ImageBridgeUnavailableNotAllowed},
		{Model: "gemini-3.1-flash-image", Reason: ImageBridgeUnavailableAccountUnschedulable},
		{Model: "grok-imagine-image-2.0", Reason: ImageBridgeUnavailableNoProvider},
	}, models.Availability)
}

func TestAPIKeyImageBridgeRebindPreservesMainKeyAndQuota(t *testing.T) {
	svc, group, user, _ := imageBridgeServiceFixture()
	main := &Group{ID: 5, Platform: PlatformOpenAI}
	original := &APIKey{ID: 77, UserID: user.ID, GroupID: &main.ID, Group: main, User: user, Quota: 25, QuotaUsed: 7}
	key, sub, err := svc.ImageBridgeAPIKey(context.Background(), original, "gemini-3.1-flash-image")
	require.NoError(t, err)
	require.Nil(t, sub)
	require.Equal(t, group.ID, *key.GroupID)
	require.Equal(t, original.ID, key.ID)
	require.Equal(t, original.QuotaUsed, key.QuotaUsed)
	require.Equal(t, original.Quota, key.Quota)
	require.Equal(t, int64(5), *original.GroupID)
	require.Same(t, main, original.Group)
	group.SubscriptionType = SubscriptionTypeSubscription
	_, _, err = svc.ImageBridgeAPIKey(context.Background(), original, "gemini-3.1-flash-image")
	require.Error(t, err)
	svc.userSubRepo.(*imageBridgeSubRepoStub).subscriptions = []UserSubscription{{ID: 3, UserID: 1, GroupID: 24}}
	_, sub, err = svc.ImageBridgeAPIKey(context.Background(), original, "gemini-3.1-flash-image")
	require.NoError(t, err)
	require.Equal(t, int64(24), sub.GroupID)
}

func TestAPIKeyImageBridgeSnapshotPreservesInheritDisableAndModel(t *testing.T) {
	empty, model := "", "gemini-3.1-flash-image"
	svc := &APIKeyService{}
	for _, selection := range []*string{nil, &empty, &model} {
		key := &APIKey{ID: 1, UserID: 2, ImageBridgeModel: selection, User: &User{ID: 2}}
		snapshot := svc.snapshotFromAPIKey(context.Background(), key)
		body, err := json.Marshal(snapshot)
		require.NoError(t, err)
		var restored APIKeyAuthSnapshot
		require.NoError(t, json.Unmarshal(body, &restored))
		require.Equal(t, selection, svc.snapshotToAPIKey("test", &restored).ImageBridgeModel)
	}
}

func TestAPIKeyImageBridgeUpdateOnlyChangesRequestedColumn(t *testing.T) {
	empty, model := "", "gemini-3.1-flash-image"
	for _, selection := range []*string{nil, &empty} {
		svc, repo := newUpdateFieldsAPIKeyService(&APIKey{ID: 1, UserID: 2, ImageBridgeModel: &model})
		key, err := svc.Update(context.Background(), 1, 2, UpdateAPIKeyRequest{ImageBridgeModel: selection, UpdateImageBridgeModel: true})
		require.NoError(t, err)
		require.Equal(t, selection, key.ImageBridgeModel)
		require.Equal(t, []APIKeyUpdateFields{{ImageBridgeModel: true}}, repo.updateFields)
	}
}

func TestAPIKeyImageBridgeRejectsNonOpenAIPrimaryGroups(t *testing.T) {
	for _, platform := range []string{PlatformGemini, PlatformComposite, PlatformAnthropic, PlatformGrok} {
		svc, _, user, _ := imageBridgeServiceFixture()
		group := &Group{ID: 5, Platform: platform}
		_, _, err := svc.ImageBridgeAPIKey(context.Background(), &APIKey{UserID: user.ID, Group: group}, DefaultGeminiImageModel)
		require.ErrorContains(t, err, "仅 OpenAI")
		model := DefaultGeminiImageModel
		svc, repo := newUpdateFieldsAPIKeyService(&APIKey{ID: 1, UserID: 2, Group: group})
		_, err = svc.Update(context.Background(), 1, 2, UpdateAPIKeyRequest{ImageBridgeModel: &model})
		require.ErrorContains(t, err, "仅 OpenAI")
		require.Empty(t, repo.updateFields)
	}
}

func TestAPIKeyImageBridgeChangingPrimaryGroupDisablesBridge(t *testing.T) {
	model := DefaultGeminiImageModel
	main := &Group{ID: 5, Platform: PlatformOpenAI}
	svc, repo := newUpdateFieldsAPIKeyService(&APIKey{ID: 1, UserID: 2, Group: main, GroupID: &main.ID, ImageBridgeModel: &model})
	other := &Group{ID: 14, Platform: PlatformGemini, Status: StatusActive}
	svc.userRepo = &imageBridgeUserRepoStub{user: &User{ID: 2}}
	svc.groupRepo = &imageBridgeGroupRepoStub{group: other}
	key, err := svc.Update(context.Background(), 1, 2, UpdateAPIKeyRequest{GroupID: &other.ID})
	require.NoError(t, err)
	require.NotNil(t, key.ImageBridgeModel)
	require.Empty(t, *key.ImageBridgeModel)
	require.Equal(t, []APIKeyUpdateFields{{GroupID: true, ImageBridgeModel: true}}, repo.updateFields)
}
