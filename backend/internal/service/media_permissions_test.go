package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mediaPermissionUsers struct {
	UserRepository
	user User
}

func (r *mediaPermissionUsers) GetByID(context.Context, int64) (*User, error) { return &r.user, nil }

type mediaPermissionGroups struct {
	GroupRepository
	groups []Group
}

func (r *mediaPermissionGroups) ListActive(context.Context) ([]Group, error) { return r.groups, nil }

type mediaPermissionSubscriptions struct {
	UserSubscriptionRepository
	active []UserSubscription
}

func (r *mediaPermissionSubscriptions) ListActiveByUserID(context.Context, int64) ([]UserSubscription, error) {
	return r.active, nil
}

type mediaPermissionDenials struct {
	UserGroupRateRepository
	err    error
	denied map[int64][]string
}

func (r *mediaPermissionDenials) GetDeniedModelsByUserID(context.Context, int64) (map[int64][]string, error) {
	if r.denied != nil {
		return r.denied, r.err
	}
	return map[int64][]string{1: {"image-denied"}}, r.err
}

func TestMediaPermissionsReusesLiveGroupAndModelPolicy(t *testing.T) {
	users := &mediaPermissionUsers{user: User{ID: 7, RestrictPublicGroups: true, AllowedGroups: []int64{1}}}
	groups := &mediaPermissionGroups{groups: []Group{
		{ID: 1, Name: "Allowed", Platform: PlatformOpenAI, ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"image-*"}}},
		{ID: 2, Name: "Unassigned", Platform: PlatformOpenAI},
		{ID: 3, Name: "Subscription", SubscriptionType: SubscriptionTypeSubscription},
	}}
	denials := &mediaPermissionDenials{}
	svc := &APIKeyService{userRepo: users, groupRepo: groups, userSubRepo: &mediaPermissionSubscriptions{}, userGroupRateRepo: denials}
	models := []MediaModelCandidate{{Model: "image-good", Kind: "image"}, {Model: "image-denied", Kind: "image"}, {Model: "video-hidden", Kind: "video"}}
	got, err := svc.MediaPermissions(context.Background(), 7, models)
	require.NoError(t, err)
	require.Equal(t, []MediaPermissionGroup{{ID: 1, Name: "Allowed", Models: []string{"image-good"}}}, got)
	users.user.AllowedGroups = nil
	got, err = svc.MediaPermissions(context.Background(), 7, models)
	require.NoError(t, err)
	require.Empty(t, got, "revoked group is rejected on the next request")
	denials.err = errors.New("policy lookup unavailable")
	_, err = svc.MediaPermissions(context.Background(), 7, models)
	require.Error(t, err, "unavailable policy must not become unrestricted access")
}

type mediaAssistantKeys struct {
	APIKeyRepository
	keys               map[int64]*APIKey
	created            []*APIKey
	deleted            []int64
	lookups            int
	listErr, deleteErr error
}

func (r *mediaAssistantKeys) GetByKeyForAuth(_ context.Context, raw string) (*APIKey, error) {
	r.lookups++
	for _, key := range r.keys {
		if key.Key == raw {
			copy := *key
			return &copy, nil
		}
	}
	return nil, ErrAPIKeyNotFound
}
func (r *mediaAssistantKeys) ListByUserID(context.Context, int64, pagination.PaginationParams, APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
	if r.listErr != nil {
		return nil, nil, r.listErr
	}
	var result []APIKey
	for _, key := range r.keys {
		result = append(result, *key)
	}
	return result, &pagination.PaginationResult{}, nil
}
func (r *mediaAssistantKeys) GetKeyAndOwnerID(_ context.Context, id int64) (string, int64, error) {
	key := r.keys[id]
	if key == nil {
		return "", 0, ErrAPIKeyNotFound
	}
	return key.Key, key.UserID, nil
}
func (r *mediaAssistantKeys) DeleteWithAudit(_ context.Context, id int64) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	r.deleted = append(r.deleted, id)
	delete(r.keys, id)
	return nil
}
func (r *mediaAssistantKeys) CountByUserID(_ context.Context, id int64) (int64, error) {
	var n int64
	for _, key := range r.keys {
		if key.UserID == id {
			n++
		}
	}
	return n, nil
}
func (r *mediaAssistantKeys) Create(_ context.Context, key *APIKey) error {
	r.created = append(r.created, key)
	return nil
}

func mediaAssistantServiceFixture() (*APIKeyService, *mediaAssistantKeys, *mediaPermissionUsers) {
	repo := &mediaAssistantKeys{keys: map[int64]*APIKey{}}
	users := &mediaPermissionUsers{user: User{ID: 7, Status: StatusActive, RestrictPublicGroups: true, AllowedGroups: []int64{1}}}
	svc := &APIKeyService{apiKeyRepo: repo, userRepo: users, groupRepo: &mediaPermissionGroups{groups: []Group{{ID: 1, Platform: PlatformOpenAI, ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"gpt-*"}}}}}, userSubRepo: &mediaPermissionSubscriptions{}, cfg: &config.Config{}}
	return svc, repo, users
}

func TestMediaAssistantKeyRevalidatesCredentialScopeAndLivePermissions(t *testing.T) {
	for _, mode := range []string{"valid", "wrong-owner", "wrong-group", "no-group", "disabled", "expired", "permanent", "outlives-session", "quota-exhausted", "revoked-group", "denied-model", "expired-session", "excessive-lifetime", "invalid-model"} {
		t.Run(mode, func(t *testing.T) {
			svc, repo, users := mediaAssistantServiceFixture()
			expiry := time.Now().Add(10 * time.Minute)
			keyExpiry := time.Now().Add(5 * time.Minute)
			groupID := int64(1)
			key := &APIKey{ID: 1, UserID: 7, GroupID: &groupID, Key: "sk-cached-private-fixture", Name: mediaAssistantKeyName, Status: StatusActive, ExpiresAt: &keyExpiry, ConcurrencyLimit: 1}
			repo.keys[1] = key
			model := "gpt-6-astra"
			switch mode {
			case "wrong-owner":
				key.UserID = 8
			case "wrong-group":
				groupID = 2
			case "no-group":
				key.GroupID = nil
			case "disabled":
				key.Status = StatusAPIKeyDisabled
			case "expired":
				keyExpiry = time.Now().Add(-time.Second)
			case "permanent":
				key.ExpiresAt = nil
			case "outlives-session":
				keyExpiry = expiry.Add(time.Second)
			case "quota-exhausted":
				key.Quota = 1
				key.QuotaUsed = 1
			case "revoked-group":
				users.user.AllowedGroups = nil
			case "denied-model":
				svc.userGroupRateRepo = &mediaPermissionDenials{denied: map[int64][]string{1: {"gpt-*"}}}
			case "expired-session":
				expiry = time.Now().Add(-time.Second)
			case "excessive-lifetime":
				expiry = time.Now().Add(17 * time.Minute)
			case "invalid-model":
				model = "gpt-6-astra\n"
			}
			got, err := svc.MediaAssistantKey(t.Context(), 7, 1, model, key.Key, expiry)
			if mode == "valid" {
				require.NoError(t, err)
				assert.Equal(t, key.Key, got.Key)
				assert.Equal(t, keyExpiry, got.ExpiresAt)
			} else {
				require.Error(t, err)
				assert.Nil(t, got)
			}
			assert.Empty(t, repo.created, "denied cached credentials must not silently mint replacements")
			assert.Empty(t, repo.deleted)
		})
	}
	svc, repo, users := mediaAssistantServiceFixture()
	groupID := int64(1)
	expiry := time.Now().Add(5 * time.Minute)
	repo.keys[1] = &APIKey{ID: 1, UserID: 7, GroupID: &groupID, Key: "sk-live-policy-fixture", Status: StatusActive, ExpiresAt: &expiry}
	_, err := svc.MediaAssistantKey(t.Context(), 7, 1, "gpt-6-astra", repo.keys[1].Key, expiry)
	require.NoError(t, err)
	users.user.AllowedGroups = nil
	_, err = svc.MediaAssistantKey(t.Context(), 7, 1, "gpt-6-astra", repo.keys[1].Key, expiry)
	require.Error(t, err)
	assert.Equal(t, 1, repo.lookups, "revoked permissions must be checked before credential cache reuse")
}

func TestMediaAssistantExpiredRecordsReleaseCountLimitWithAudit(t *testing.T) {
	svc, repo, _ := mediaAssistantServiceFixture()
	svc.cfg.APIKeyCreate.MaxActivePerUser = 3
	expired, future := time.Now().Add(-time.Minute), time.Now().Add(5*time.Minute)
	repo.keys = map[int64]*APIKey{
		1: {ID: 1, UserID: 7, Key: "sk-expired-auto-fixture", Name: mediaAssistantKeyName, ConcurrencyLimit: 1, ExpiresAt: &expired},
		2: {ID: 2, UserID: 7, Key: "sk-expired-manual-fixture", Name: "manual key", ConcurrencyLimit: 1, ExpiresAt: &expired},
		3: {ID: 3, UserID: 7, Key: "sk-active-auto-fixture", Name: mediaAssistantKeyName, ConcurrencyLimit: 1, ExpiresAt: &future},
		4: {ID: 4, UserID: 8, Key: "sk-foreign-auto-fixture", Name: mediaAssistantKeyName, ConcurrencyLimit: 1, ExpiresAt: &expired},
	}
	expiry := time.Now().Add(10 * time.Minute)
	got, err := svc.MediaAssistantKey(t.Context(), 7, 1, "gpt-6-astra", "", expiry)
	require.NoError(t, err)
	assert.Equal(t, []int64{1}, repo.deleted, "only expired automatic records owned by this user may be tombstoned")
	require.Len(t, repo.created, 1)
	created := repo.created[0]
	assert.Equal(t, expiry, *created.ExpiresAt)
	assert.Equal(t, got.Key, created.Key)
	assert.Equal(t, int64(7), created.UserID)
	assert.Equal(t, int64(1), *created.GroupID)
	assert.Equal(t, 1, created.ConcurrencyLimit)
	assert.Equal(t, mediaAssistantKeyName, created.Name)
	assert.Contains(t, repo.keys, int64(2))
	assert.Contains(t, repo.keys, int64(3))
	assert.Contains(t, repo.keys, int64(4))
	for _, mode := range []string{"list-fails", "audit-delete-fails", "count-limit"} {
		t.Run(mode, func(t *testing.T) {
			svc, repo, _ := mediaAssistantServiceFixture()
			repo.keys[1] = &APIKey{ID: 1, UserID: 7, Name: mediaAssistantKeyName, ConcurrencyLimit: 1, ExpiresAt: &expired}
			switch mode {
			case "list-fails":
				repo.listErr = errors.New("database unavailable")
			case "audit-delete-fails":
				repo.deleteErr = errors.New("audit unavailable")
			case "count-limit":
				repo.keys[1].Name = "manual key"
				svc.cfg.APIKeyCreate.MaxActivePerUser = 1
			}
			_, err := svc.MediaAssistantKey(t.Context(), 7, 1, "gpt-6-astra", "", expiry)
			require.Error(t, err)
			assert.Empty(t, repo.created)
		})
	}
}

func TestMediaPermissionsTextPlatformAndScopesWithDisabledAllowlist(t *testing.T) {
	gpt := []string{"gpt-5.6-sol", "gpt-5.6-luna", "gpt-6-astra", "gpt-6"}
	claude, gemini := "claude-sonnet-4-5", "gemini-2.5-pro"
	candidates := make([]MediaModelCandidate, 0, len(gpt)+3)
	for _, model := range append(append([]string{}, gpt...), claude, gemini, "gpt-image-1", "unrecognized-model") {
		candidates = append(candidates, MediaModelCandidate{Model: model, Kind: "text"})
	}
	for _, tc := range []struct {
		name  string
		group Group
		want  []string
	}{
		{name: "openai", group: Group{Platform: PlatformOpenAI}, want: gpt},
		{name: "openai-legacy-default-scopes", group: Group{Platform: PlatformOpenAI, SupportedModelScopes: []string{"claude", "gemini_text", "gemini_image"}}, want: gpt},
		{name: "disabled-allowlist-is-not-a-platform-grant", group: Group{Platform: PlatformAnthropic, ModelAllowlist: GroupModelAllowlist{Models: []string{"gpt-*"}}}, want: []string{claude}},
		{name: "video-even-explicit-gpt-allowlist", group: Group{Platform: PlatformVideo, ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"gpt-*"}}}},
		{name: "image-only-openai-allowlist", group: Group{Platform: PlatformOpenAI, ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"gpt-image-*"}}}},
		{name: "anthropic", group: Group{Platform: PlatformAnthropic}, want: []string{claude}},
		{name: "gemini", group: Group{Platform: PlatformGemini}, want: []string{gemini}},
		{name: "antigravity-unrestricted", group: Group{Platform: PlatformAntigravity}, want: []string{claude, gemini}},
		{name: "antigravity-claude", group: Group{Platform: PlatformAntigravity, SupportedModelScopes: []string{"claude"}}, want: []string{claude}},
		{name: "antigravity-gemini-text", group: Group{Platform: PlatformAntigravity, SupportedModelScopes: []string{"gemini_text"}}, want: []string{gemini}},
		{name: "antigravity-image-only", group: Group{Platform: PlatformAntigravity, AllowImageGeneration: true, SupportedModelScopes: []string{"gemini_image"}}},
		{name: "antigravity-unknown-scope", group: Group{Platform: PlatformAntigravity, SupportedModelScopes: []string{"unknown"}}},
		{name: "unknown-platform"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Deliberately use the same misleading name and unrelated ID in every
			// case: only persisted policy and platform may affect authorization.
			tc.group.ID, tc.group.Name = 901, "OpenAI Claude image video"
			svc := &APIKeyService{userRepo: &mediaPermissionUsers{user: User{ID: 7}}, groupRepo: &mediaPermissionGroups{groups: []Group{tc.group}}, userSubRepo: &mediaPermissionSubscriptions{}}
			got, err := svc.MediaPermissions(t.Context(), 7, candidates)
			require.NoError(t, err)
			if len(tc.want) == 0 {
				assert.Empty(t, got)
			} else {
				require.Len(t, got, 1)
				assert.Equal(t, tc.want, got[0].Models)
			}
		})
	}
}

func TestMediaAssistantKeyRejectsWrongTextPlatformBeforeCredentialLookup(t *testing.T) {
	for _, platform := range []string{PlatformAnthropic, PlatformAntigravity, PlatformVideo} {
		for _, cached := range []string{"", "sk-cached-private-fixture"} {
			t.Run(platform+"/"+cached, func(t *testing.T) {
				svc, repo, _ := mediaAssistantServiceFixture()
				svc.groupRepo = &mediaPermissionGroups{groups: []Group{{ID: 1, Platform: platform}}}
				got, err := svc.MediaAssistantKey(t.Context(), 7, 1, "gpt-6-astra", cached, time.Now().Add(5*time.Minute))
				require.ErrorIs(t, err, ErrGroupNotAllowed)
				assert.Nil(t, got)
				assert.Zero(t, repo.lookups)
				assert.Empty(t, repo.created)
				assert.Empty(t, repo.deleted)
			})
		}
	}
}
