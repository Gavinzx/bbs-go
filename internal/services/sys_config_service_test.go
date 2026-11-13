package services

import (
	"testing"

	"bbs-go/internal/cache"
	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
)

func TestParseModulesConfig_BackfillsQaFromTopicForLegacyConfig(t *testing.T) {
	cfg := parseModulesConfig(`{"tweet":true,"topic":true,"article":false}`)

	if !cfg.QA {
		t.Fatalf("expected legacy config without qa to keep QA enabled when topic is enabled")
	}
}

func TestParseModulesConfig_RespectsExplicitQaSwitch(t *testing.T) {
	cfg := parseModulesConfig(`{"tweet":true,"topic":true,"qa":false,"article":true}`)

	if cfg.QA {
		t.Fatalf("expected explicit qa=false to disable QA independently from topic")
	}
}

func TestDefaultNotificationTypeKeysDropRemovedCommunityGrowthEvents(t *testing.T) {
	keys := defaultNotificationTypeKeys()
	for _, removed := range []string{"topicFavorite", "userLevelUp", "userBadgeGrant"} {
		if containsString(keys, removed) {
			t.Fatalf("expected notification type %q to be removed, got %#v", removed, keys)
		}
	}
	for _, expected := range []string{"topicComment", "commentReply", "topicLike", "topicRecommend", "topicDelete", "articleComment", "qaAnswerAccepted"} {
		if !containsString(keys, expected) {
			t.Fatalf("expected notification type %q to remain, got %#v", expected, keys)
		}
	}
}

func TestGetLoginConfigRegisterEnabledDefaultsToTrue(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&models.SysConfig{}); err != nil {
		t.Fatalf("auto migrate sys config: %v", err)
	}
	cache.SysConfigCache.Invalidate(constants.SysConfigLoginConfig)
	t.Cleanup(func() {
		cache.SysConfigCache.Invalidate(constants.SysConfigLoginConfig)
	})

	if err := SysConfigService.Set(constants.SysConfigLoginConfig, `{"passwordLogin":{"enabled":true}}`); err != nil {
		t.Fatalf("set login config: %v", err)
	}
	cfg := SysConfigService.GetLoginConfig()
	if !cfg.IsRegisterEnabled() {
		t.Fatalf("expected legacy login config without registerEnabled to keep registration open")
	}
	if cfg.RegisterEnabled == nil || !*cfg.RegisterEnabled {
		t.Fatalf("expected registerEnabled to be normalized to true")
	}
}

func TestGetLoginConfigRegisterEnabledCanBeDisabled(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&models.SysConfig{}); err != nil {
		t.Fatalf("auto migrate sys config: %v", err)
	}
	cache.SysConfigCache.Invalidate(constants.SysConfigLoginConfig)
	t.Cleanup(func() {
		cache.SysConfigCache.Invalidate(constants.SysConfigLoginConfig)
	})

	if err := SysConfigService.Set(constants.SysConfigLoginConfig, `{"registerEnabled":false,"passwordLogin":{"enabled":true}}`); err != nil {
		t.Fatalf("set login config: %v", err)
	}
	if SysConfigService.GetLoginConfig().IsRegisterEnabled() {
		t.Fatalf("expected registration to be disabled")
	}
}
