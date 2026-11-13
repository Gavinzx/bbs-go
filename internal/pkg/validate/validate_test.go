package validate

import (
	"strings"
	"testing"

	"bbs-go/internal/pkg/config"
)

func init() {
	config.Instance = &config.Config{Language: config.LanguageZhCN}
}

func TestIsUsernameAllowsLettersAndUnderscore(t *testing.T) {
	valid := []string{"zhang_san", "user_01", "username", "abcdef", "a_b_c", "abcdefghijklm", "a1234567890123456789"}
	for _, username := range valid {
		if err := IsUsername(username); err != nil {
			t.Fatalf("expected %q to be valid, got %v", username, err)
		}
	}
}

func TestIsUsernameRejectsDigitsHyphensAndInvalidStarts(t *testing.T) {
	invalid := []string{"zhang-san", "1zhang", "_zhang", "abc", "abcdefghijklmnopqrstu", "张三"}
	for _, username := range invalid {
		if err := IsUsername(username); err == nil {
			t.Fatalf("expected %q to be invalid", username)
		}
	}
}

func TestIsPasswordRejectsBcryptOverflow(t *testing.T) {
	if err := IsPassword("a123456"); err != nil {
		t.Fatalf("expected short password to be valid: %v", err)
	}
	if err := IsPassword("a" + strings.Repeat("b", 71)); err != nil {
		t.Fatalf("expected 72-byte password to be valid: %v", err)
	}
	if err := IsPassword("a" + strings.Repeat("b", 72)); err == nil {
		t.Fatalf("expected 73-byte password to be rejected")
	}
}
