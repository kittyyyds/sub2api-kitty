package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/vmbridge"
)

// Entries are transient input, never part of a job or a persisted policy.
type OpenAIVMBatchEntry struct {
	Email     string `json:"email"`
	Password  string `json:"password"`
	MFASecret string `json:"mfa_secret"`
}

func openAIVMBatchInputLines(in OpenAIVMBatchInput) ([]OpenAIVMBatchLine, []OpenAIVMBatchLineError, error) {
	if in.Count == 0 && len(in.Entries) == 0 && in.LoginMode == "" {
		lines, problems := ParseOpenAIVMBatch(in.Text)
		return lines, problems, nil
	}
	bad := func() error {
		return infraerrors.BadRequest("OPENAI_VM_BATCH_COUNT", "Choose 1 to 20 KVMs and provide exactly that many selected accounts")
	}
	if in.Count < 1 || in.Count > OpenAIVMBatchMaxLines || len(in.NamePrefix) > 180 || strings.ContainsAny(in.NamePrefix, "\r\n") {
		return nil, nil, bad()
	}
	if in.LoginMode != "manual" && in.LoginMode != "batch_2fa" {
		return nil, nil, bad()
	}
	if in.LoginMode == "batch_2fa" && (len(in.Entries) != in.Count || strings.TrimSpace(in.Text) != "") {
		return nil, nil, bad()
	}
	if in.LoginMode == "manual" && (len(in.Entries) > 0 || strings.TrimSpace(in.Text) != "" || strings.TrimSpace(in.NamePrefix) == "") {
		return nil, nil, bad()
	}
	lines := make([]OpenAIVMBatchLine, 0, in.Count)
	problems := []OpenAIVMBatchLineError{}
	seen := map[string]bool{}
	for i := 0; i < in.Count; i++ {
		line := OpenAIVMBatchLine{LineNo: i + 1}
		if prefix := strings.TrimSpace(in.NamePrefix); prefix != "" {
			line.Name = prefix
			if in.Count > 1 {
				line.Name = fmt.Sprintf("%s-%02d", prefix, i+1)
			}
		}
		if in.LoginMode == "batch_2fa" {
			entry := in.Entries[i]
			line.Email, line.Password = strings.TrimSpace(entry.Email), entry.Password
			line.MFASecret = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(entry.MFASecret), " ", ""))
			code := ""
			switch {
			case !validOpenAIVMBatchEmail(line.Email):
				code = "invalid_email"
			case strings.TrimSpace(line.Password) == "" || len(line.Password) > 4096:
				code = "missing_password"
			case !validOpenAIVMBatchMFASecret(line.MFASecret):
				code = "invalid_mfa_secret"
			case seen[strings.ToLower(line.Email)]:
				code = "duplicate_in_batch"
			}
			seen[strings.ToLower(line.Email)] = true
			if code != "" {
				problems = append(problems, OpenAIVMBatchLineError{LineNo: i + 1, Email: openAIVMBatchSafeEmail(line.Email), Code: code})
				continue
			}
		}
		lines = append(lines, line)
	}
	return lines, problems, nil
}

// Tie-breaking follows the selected order, which the UI may shuffle once.
// Existing load is included. No retry or request-time rotation changes a slot's exit.
func AssignOpenAIVMBalancedProxies(lines []OpenAIVMBatchLine, ids []int64, used map[int64]int, cap int) (map[int]int64, []int) {
	limits := map[int64]int{}
	for _, id := range ids {
		limits[id] = cap
	}
	return assignOpenAIVMBalancedWithLimits(lines, ids, used, limits)
}

func assignOpenAIVMBalancedWithLimits(lines []OpenAIVMBatchLine, ids []int64, used, limits map[int64]int) (map[int]int64, []int) {
	assigned := map[int]int64{}
	unplaced := []int{}
	load := map[int64]int{}
	for id, n := range used {
		load[id] = n
	}
	for _, line := range lines {
		var chosen int64
		for _, id := range ids {
			if load[id] < limits[id] && (chosen == 0 || load[id] < load[chosen]) {
				chosen = id
			}
		}
		if chosen == 0 {
			unplaced = append(unplaced, line.LineNo)
			continue
		}
		assigned[line.LineNo] = chosen
		load[chosen]++
	}
	return assigned, unplaced
}

// Multiple Custom records can reference one VM2 proxy. Count their bindings
// together and use the first selected record, never multiply its capacity.
func uniqueVMProxyPools(ids []int64, proxies []Proxy, used map[int64]int) ([]int64, map[int64]int) {
	type tuple struct {
		host, username, password string
		port                     int
	}
	byID := map[int64]tuple{}
	poolLoad := map[tuple]int{}
	for _, p := range proxies {
		if p.Protocol != "socks5" && p.Protocol != "socks5h" {
			continue
		}
		key := tuple{strings.ToLower(strings.Trim(p.Host, "[]")), p.Username, p.Password, p.Port}
		byID[p.ID] = key
		poolLoad[key] += used[p.ID]
	}
	seen := map[tuple]bool{}
	unique := []int64{}
	load := map[int64]int{}
	for _, id := range ids {
		key, ok := byID[id]
		if ok && seen[key] {
			continue
		}
		if ok {
			seen[key] = true
			load[id] = poolLoad[key]
		} else {
			load[id] = used[id]
		}
		unique = append(unique, id)
	}
	return unique, load
}

func (s *OpenAIVMBatchService) proxyCapacity(ctx context.Context, ids []int64) ([]vmbridge.ProxyCapacity, error) {
	if len(ids) == 0 || len(ids) > 32 {
		return nil, infraerrors.BadRequest("OPENAI_VM_PROXY_REQUIRED", "Select 1 to 32 SOCKS5 proxies")
	}
	c, key, err := s.managed.vm.configured(ctx)
	if err != nil {
		return nil, err
	}
	urls := make([]string, 0, len(ids))
	for _, id := range ids {
		p, e := s.managed.socksProxy(ctx, id)
		if e != nil {
			return nil, e
		}
		urls = append(urls, p.URL())
	}
	var out []vmbridge.ProxyCapacity
	if err = s.managed.vm.bridgeJSON(ctx, c, key, http.MethodPost, "/v1/managed-capacity", map[string]any{"proxies": urls}, &out); err != nil {
		return nil, infraerrors.BadRequest("OPENAI_VM_CAPACITY_UNAVAILABLE", "Unable to check VM2 proxy capacity. Verify that the matching Bridge is running")
	}
	if len(out) != len(ids) {
		return nil, infraerrors.BadRequest("OPENAI_VM_CAPACITY_UNAVAILABLE", "Invalid VM2 proxy capacity response")
	}
	return out, nil
}
