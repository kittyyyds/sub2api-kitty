package service

import "testing"

func TestAccountOpenAIGPT5VariantLock(t *testing.T) {
	base := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	if got := base.GetOpenAIGPT5VariantLock(); got != OpenAIGPT5VariantLockAuto {
		t.Fatalf("default lock = %q, want auto", got)
	}

	base.Extra = map[string]any{"openai_gpt5_variant_lock": " TERRA "}
	if got := base.GetOpenAIGPT5VariantLock(); got != OpenAIGPT5VariantLockTerra {
		t.Fatalf("normalized lock = %q, want terra", got)
	}
	if got := base.ApplyOpenAIGPT5VariantLock("gpt-5.6-sol"); got != "gpt-5.6-terra" {
		t.Fatalf("locked variant = %q, want gpt-5.6-terra", got)
	}
	if got := base.ApplyOpenAIGPT5VariantLock("gpt-5.5"); got != "gpt-5.5" {
		t.Fatalf("non-5.6 model changed to %q", got)
	}

	apiKey := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: base.Extra}
	if got := apiKey.ApplyOpenAIGPT5VariantLock("gpt-5.6-sol"); got != "gpt-5.6-sol" {
		t.Fatalf("API key variant changed to %q", got)
	}
}

func TestNormalizeOpenAIModelForUpstreamAppliesGPT5VariantLock(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"openai_gpt5_variant_lock": "luna"},
	}
	if got := normalizeOpenAIModelForUpstream(account, "gpt-5.6"); got != "gpt-5.6-luna" {
		t.Fatalf("normalized model = %q, want gpt-5.6-luna", got)
	}
}
