package repository

import (
	"context"

	"github.com/redis/go-redis/v9"
)

var clearUndeliveredCodeScript = redis.NewScript(`
local v = redis.call('GET', KEYS[1])
if not v then return 0 end
local ok, d = pcall(cjson.decode, v)
if not ok or type(d) ~= 'table' or d['Code'] ~= ARGV[1] then return 0 end
redis.call('DEL', KEYS[1], KEYS[2])
return 1
`)

func (c *emailCache) clearUndeliveredCode(ctx context.Context, key, code string) error {
	return clearUndeliveredCodeScript.Run(ctx, c.rdb, []string{key, key + attemptsKeySuffix}, code).Err()
}

func (c *emailCache) DeleteUndeliveredVerificationCode(ctx context.Context, email, code string) error {
	return c.clearUndeliveredCode(ctx, verifyCodeKey(email), code)
}

func (c *emailCache) DeleteUndeliveredNotifyCode(ctx context.Context, email, code string) error {
	return c.clearUndeliveredCode(ctx, notifyVerifyKey(email), code)
}
