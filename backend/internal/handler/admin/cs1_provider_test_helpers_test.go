package admin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"strings"
	"sync"
)

type providerTestEncryptor struct{}

func (providerTestEncryptor) Encrypt(v string) (string, error) {
	return "encrypted:" + base64.StdEncoding.EncodeToString([]byte(v)), nil
}
func (providerTestEncryptor) Decrypt(v string) (string, error) {
	b, e := base64.StdEncoding.DecodeString(strings.TrimPrefix(v, "encrypted:"))
	return string(b), e
}

type providerMemoryStore struct {
	mu      sync.Mutex
	records []providerRecord
}

func (r *providerMemoryStore) CS1UserBalance(context.Context) (float64, error) { return 0, nil }
func (r *providerMemoryStore) LoadProviderRecords(_ context.Context, out any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, e := json.Marshal(r.records)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, out)
}
func (r *providerMemoryStore) SaveCS1Provider(_ context.Context, id string, revision int64, in any, out any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, e := json.Marshal(in)
	if e != nil {
		return e
	}
	var v providerRecord
	if e = json.Unmarshal(b, &v); e != nil {
		return e
	}
	found := -1
	for i := range r.records {
		if r.records[i].ID == id {
			found = i
			break
		}
	}
	if (found < 0 && revision != 0) || (found >= 0 && r.records[found].Revision != revision) {
		return service.ErrProviderHubConflict
	}
	v.Revision = revision + 1
	if found < 0 {
		r.records = append(r.records, v)
	} else {
		r.records[found] = v
	}
	b, e = json.Marshal(v)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, out)
}
func (r *providerMemoryStore) DeleteCS1Provider(_ context.Context, id string, revision int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, v := range r.records {
		if v.ID == id && v.Revision == revision {
			r.records = append(r.records[:i], r.records[i+1:]...)
			return nil
		}
	}
	return service.ErrProviderHubConflict
}
