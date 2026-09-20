package cache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrLockNotAcquired is returned when the lock could not be taken within the
// allotted wait window because another operation still holds it.
var ErrLockNotAcquired = errors.New("could not acquire lock")

// releaseScript deletes the lock key only if it still holds our token. This
// guards against releasing a lock that already expired and was re-acquired by
// another operation: without the token check we could delete someone else's
// lock. Running it as a Lua script makes the get-and-delete atomic.
var releaseScript = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("del", KEYS[1])
else
	return 0
end
`)

// retryDelay is how long AcquireLock sleeps between contention retries.
const retryDelay = 50 * time.Millisecond

// Lock is a held distributed lock. Call Release to give it up.
type Lock struct {
	client *redis.Client
	key    string
	token  string
}

// AcquireLock takes the lock at key, retrying until it succeeds, the wait
// window elapses (ErrLockNotAcquired), or ctx is cancelled. The lock
// auto-expires after ttl so a crashed holder cannot block the key forever.
func (c *Cache) AcquireLock(ctx context.Context, key string, ttl, wait time.Duration) (*Lock, error) {
	token, err := randomToken()
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(wait)
	for {
		ok, err := c.client.SetNX(ctx, key, token, ttl).Result()
		if err != nil {
			return nil, err
		}
		if ok {
			return &Lock{client: c.client, key: key, token: token}, nil
		}

		if time.Now().After(deadline) {
			return nil, ErrLockNotAcquired
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(retryDelay):
		}
	}
}

// Release relinquishes the lock. It is safe to call even if the lock has
// already expired: the token check simply makes it a no-op in that case.
func (l *Lock) Release(ctx context.Context) error {
	return releaseScript.Run(ctx, l.client, []string{l.key}, l.token).Err()
}

// randomToken returns a unique, unguessable owner token for a lock.
func randomToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
