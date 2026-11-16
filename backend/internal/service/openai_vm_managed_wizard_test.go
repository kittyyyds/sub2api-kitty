package service

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestVMWizardBalancedAllocationAndCapacity(t *testing.T) {
	for _, tc := range []struct {
		n, p int
		want []int
	}{{5, 3, []int{2, 2, 1}}, {6, 2, []int{3, 3}}, {6, 4, []int{2, 2, 1, 1}}, {5, 1, []int{5}}} {
		lines := make([]OpenAIVMBatchLine, tc.n)
		ids := make([]int64, tc.p)
		for i := range lines {
			lines[i].LineNo = i + 1
		}
		for i := range ids {
			ids[i] = int64(i + 1)
		}
		assigned, unplaced := AssignOpenAIVMBalancedProxies(lines, ids, nil, 5)
		require.Empty(t, unplaced)
		load := make([]int, tc.p)
		for _, id := range assigned {
			load[id-1]++
		}
		require.Equal(t, tc.want, load)
	}
	lines := []OpenAIVMBatchLine{{LineNo: 1}, {LineNo: 2}, {LineNo: 3}}
	assigned, unplaced := assignOpenAIVMBalancedWithLimits(lines, []int64{1, 2}, map[int64]int{1: 4, 2: 5}, map[int64]int{1: 5, 2: 5})
	require.Equal(t, map[int]int64{1: 1}, assigned)
	require.Equal(t, []int{2, 3}, unplaced)
}

func TestVMWizardProxyAliasesShareCapacity(t *testing.T) {
	proxies := []Proxy{{ID: 1, Protocol: "socks5", Host: "EXIT.example", Port: 1080, Username: "u", Password: "p"}, {ID: 2, Protocol: "socks5h", Host: "exit.example", Port: 1080, Username: "u", Password: "p"}, {ID: 3, Protocol: "socks5", Host: "exit.example", Port: 1080, Username: "other", Password: "p"}}
	ids, used := uniqueVMProxyPools([]int64{2, 1, 3}, proxies, map[int64]int{1: 2, 2: 1, 3: 1})
	require.Equal(t, []int64{2, 3}, ids)
	require.Equal(t, map[int64]int{2: 3, 3: 1}, used)
}

func TestVMWizardKeepsPasswordDashesAndStructuredEntries(t *testing.T) {
	for _, sep := range []string{"---", "----"} {
		line := "u@example.test" + sep + "p----with---dashes" + sep + batchSecret
		rows, problems := ParseOpenAIVMBatch(line)
		require.Empty(t, problems)
		require.Equal(t, "p----with---dashes", rows[0].Password)
	}
	in := OpenAIVMBatchInput{Count: 2, LoginMode: "batch_2fa", Entries: []OpenAIVMBatchEntry{{Email: "u@example.test", Password: " ---pw---- ", MFASecret: batchSecret}, {Email: "v@example.test", Password: "pw2", MFASecret: batchSecret}}}
	rows, problems, err := openAIVMBatchInputLines(in)
	require.NoError(t, err)
	require.Empty(t, problems)
	require.Equal(t, " ---pw---- ", rows[0].Password)
	in.Count = 1
	_, _, err = openAIVMBatchInputLines(in)
	require.Error(t, err)
	in.Count = 2
	in.Entries[1].Email = "U@example.test"
	_, problems, err = openAIVMBatchInputLines(in)
	require.NoError(t, err)
	require.Equal(t, "duplicate_in_batch", problems[0].Code)
}

func TestVMWizardPolicyAppliedWithDeepCopiesAndNoLoginCredentials(t *testing.T) {
	rate, groupRate := 0.2, 0.3
	load := 7
	settings := &OpenAIVMManagedSettings{Concurrency: 5, Priority: 4, GroupIDs: []int64{8}, RateMultiplier: &rate, GroupRateMultiplier: &groupRate, LoadFactor: &load, Credentials: map[string]any{"model_mapping": map[string]any{"client": "upstream"}}, Extra: map[string]any{"cost_multiplier": 0.4}}
	require.NoError(t, settings.validate())
	a, b := &CreateAccountInput{}, &CreateAccountInput{}
	settings.apply(a)
	settings.apply(b)
	require.Equal(t, 5, a.Concurrency)
	require.Equal(t, 0.2, *a.RateMultiplier)
	require.Equal(t, 0.3, *a.GroupRateMultiplier)
	require.Equal(t, []int64{8}, a.GroupIDs)
	a.Credentials["model_mapping"].(map[string]any)["client"] = "changed"
	require.Equal(t, "upstream", b.Credentials["model_mapping"].(map[string]any)["client"])
	for _, key := range []string{"access_token", "refresh_token", "base_url", "password", "mfa_secret"} {
		settings.Credentials[key] = "canary"
		require.Error(t, settings.validate())
		delete(settings.Credentials, key)
	}
	settings.Extra["openai_vm_managed"] = false
	require.Error(t, settings.validate())
}

func TestVMWizardManualBatchWaitsForLoginAndRepeatedSubmitCreatesNothing(t *testing.T) {
	f := newBatchFixture(t)
	ctx := context.Background()
	in := OpenAIVMBatchInput{Count: 5, LoginMode: "manual", NamePrefix: "pool", ProxyIDs: []int64{41, 42}, PerProxy: 32, Settings: &OpenAIVMManagedSettings{Concurrency: 6, Priority: 3, GroupIDs: []int64{9}, Extra: map[string]any{"cost_multiplier": 0.25}}}
	preview, err := f.svc.Preview(ctx, in)
	require.NoError(t, err)
	require.Equal(t, 5, preview.Importable)
	require.NotEmpty(t, preview.PlanToken)
	_, err = f.svc.Start(ctx, 1, in)
	require.Error(t, err)
	in.PlanToken = preview.PlanToken
	job, err := f.svc.Start(ctx, 1, in)
	require.NoError(t, err)
	done := f.wait(t)
	require.Len(t, done.Items, 5)
	for _, item := range done.Items {
		require.Equal(t, "awaiting_login", item.Status)
		a, _ := f.accounts.GetByID(ctx, *item.AccountID)
		require.Equal(t, 6, a.Concurrency)
		require.Equal(t, 0.25, a.Extra["cost_multiplier"])
		require.Equal(t, []int64{9}, a.GroupIDs)
	}
	for _, req := range f.bridge.requests {
		require.NotContains(t, req, "login/password")
	}
	again, err := f.svc.Start(ctx, 1, in)
	require.NoError(t, err)
	require.Equal(t, job.ID, again.ID)
	require.Equal(t, int64(5), f.accounts.nextID)
}

func TestVMWizardChangedPreviewAndShortCapacityFailBeforeCreating(t *testing.T) {
	f := newBatchFixture(t)
	ctx := context.Background()
	in := OpenAIVMBatchInput{Count: 6, LoginMode: "manual", NamePrefix: "pool", ProxyIDs: []int64{41}, PerProxy: 32}
	preview, err := f.svc.Preview(ctx, in)
	require.NoError(t, err)
	require.Equal(t, 5, preview.Importable)
	in.PlanToken = preview.PlanToken
	_, err = f.svc.Start(ctx, 1, in)
	require.Error(t, err)
	require.Zero(t, f.accounts.nextID)
	in.Count = 1
	preview, err = f.svc.Preview(ctx, in)
	require.NoError(t, err)
	in.PlanToken = preview.PlanToken
	in.NamePrefix = "changed"
	_, err = f.svc.Start(ctx, 1, in)
	require.Error(t, err)
	require.Zero(t, f.accounts.nextID)
}

func TestVMWizardBatchLoginsPersistOnlyNonSecretProgress(t *testing.T) {
	f := newBatchFixture(t)
	ctx := context.Background()
	in := OpenAIVMBatchInput{Count: 2, LoginMode: "batch_2fa", ProxyIDs: []int64{41, 42}, PerProxy: 32, Entries: []OpenAIVMBatchEntry{{Email: "a@example.test", Password: "private-password", MFASecret: batchSecret}, {Email: "b@example.test", Password: "other-password", MFASecret: batchSecret}}}
	preview, err := f.svc.Preview(ctx, in)
	require.NoError(t, err)
	in.PlanToken = preview.PlanToken
	_, err = f.svc.Start(ctx, 1, in)
	require.NoError(t, err)
	job := f.wait(t)
	for _, item := range job.Items {
		require.Equal(t, "succeeded", item.Status)
	}
	raw, _ := json.Marshal(job)
	stored := string(raw) + f.repo.options + strings.Join(f.repo.writes, "")
	for _, value := range []string{"private-password", "other-password", batchSecret} {
		require.NotContains(t, stored, value)
	}
}
