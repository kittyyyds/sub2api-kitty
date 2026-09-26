package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/managedinstance"
	"github.com/redis/go-redis/v9"
)

const managedLeaderKey = "kitty:managed:background-leader:v1"

type managedLeaseStore struct{ rdb *redis.Client }

func (s managedLeaseStore) Acquire(ctx context.Context, owner string, ttl time.Duration) (bool, error) {
	return s.rdb.SetNX(ctx, managedLeaderKey, owner, ttl).Result()
}
func (s managedLeaseStore) Renew(ctx context.Context, owner string, ttl time.Duration) (bool, error) {
	n, err := s.rdb.Eval(ctx, `if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('PEXPIRE',KEYS[1],ARGV[2]) end return 0`, []string{managedLeaderKey}, owner, ttl.Milliseconds()).Int()
	return n == 1, err
}
func (s managedLeaseStore) Release(ctx context.Context, owner string) error {
	return s.rdb.Eval(ctx, `if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1]) end return 0`, []string{managedLeaderKey}, owner).Err()
}

func configureManagedLeaseStore(rdb *redis.Client) {
	if managedinstance.Enabled() {
		managedinstance.ConfigureLeaseStore(managedLeaseStore{rdb})
	}
}
