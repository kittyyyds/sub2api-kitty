package service

import "testing"

func TestGetProxyPoolPrefersConfiguredPoolAndDeduplicates(t *testing.T) {
	legacy := int64(99)
	a := &Account{ProxyID: &legacy, Extra: map[string]any{"proxy_pool": []any{float64(2), "3", float64(2), 0}}}
	got := a.GetProxyPool()
	if len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Fatalf("GetProxyPool() = %#v, want [2 3]", got)
	}
}

func TestGetProxyPoolFallsBackToLegacyIDsAndProxyID(t *testing.T) {
	legacy := int64(7)
	if got := (&Account{ProxyID: &legacy, Extra: map[string]any{"extra_proxy_ids": []any{"8"}}}).GetProxyPool(); len(got) != 1 || got[0] != 8 {
		t.Fatalf("legacy extra fallback = %#v, want [8]", got)
	}
	if got := (&Account{ProxyID: &legacy}).GetProxyPool(); len(got) != 1 || got[0] != 7 {
		t.Fatalf("proxy_id fallback = %#v, want [7]", got)
	}
}

func TestGetProxyPoolExplicitEmptyPoolDoesNotFallBack(t *testing.T) {
	legacy := int64(7)
	a := &Account{ProxyID: &legacy, Extra: map[string]any{"proxy_pool": []any{}}}
	if got := a.GetProxyPool(); len(got) != 1 || got[0] != 7 {
		t.Fatalf("read-time empty pool fallback = %#v, want legacy [7]", got)
	}
}
