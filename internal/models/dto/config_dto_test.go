package dto

import "testing"

func TestLoginConfigIsRegisterEnabled(t *testing.T) {
	enabled := true
	disabled := false

	tests := []struct {
		name string
		cfg  LoginConfig
		want bool
	}{
		{name: "nil defaults to enabled", cfg: LoginConfig{}, want: true},
		{name: "explicit enabled", cfg: LoginConfig{RegisterEnabled: &enabled}, want: true},
		{name: "explicit disabled", cfg: LoginConfig{RegisterEnabled: &disabled}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.IsRegisterEnabled(); got != tt.want {
				t.Fatalf("IsRegisterEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}
