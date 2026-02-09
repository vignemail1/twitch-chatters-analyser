package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client wraps redis client with helper methods
type Client struct {
	client *redis.Client
}

// NewClient creates a new Redis client
func NewClient(url string) (*Client, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("invalid redis URL: %w", err)
	}

	client := redis.NewClient(opt)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return &Client{client: client}, nil
}

// Close closes the Redis connection
func (c *Client) Close() error {
	return c.client.Close()
}

// --- Cache methods ---

// Get retrieves a value from cache
func (c *Client) Get(ctx context.Context, key string) (string, error) {
	return c.client.Get(ctx, key).Result()
}

// Set stores a value in cache with TTL
func (c *Client) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}

// GetJSON retrieves and unmarshals JSON from cache
func (c *Client) GetJSON(ctx context.Context, key string, dest interface{}) error {
	val, err := c.client.Get(ctx, key).Result()
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(val), dest)
}

// SetJSON marshals and stores JSON in cache with TTL
func (c *Client) SetJSON(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key, data, ttl).Err()
}

// Delete removes a key from cache
func (c *Client) Delete(ctx context.Context, keys ...string) error {
	return c.client.Del(ctx, keys...).Err()
}

// Exists checks if a key exists
func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	n, err := c.client.Exists(ctx, key).Result()
	return n > 0, err
}

// --- Session methods ---

// SetSession stores session data with expiration
func (c *Client) SetSession(ctx context.Context, sessionID string, data map[string]interface{}, ttl time.Duration) error {
	return c.SetJSON(ctx, "session:"+sessionID, data, ttl)
}

// GetSession retrieves session data
func (c *Client) GetSession(ctx context.Context, sessionID string, dest interface{}) error {
	return c.GetJSON(ctx, "session:"+sessionID, dest)
}

// DeleteSession removes a session
func (c *Client) DeleteSession(ctx context.Context, sessionID string) error {
	return c.Delete(ctx, "session:"+sessionID)
}

// RefreshSessionTTL extends session expiration
func (c *Client) RefreshSessionTTL(ctx context.Context, sessionID string, ttl time.Duration) error {
	return c.client.Expire(ctx, "session:"+sessionID, ttl).Err()
}

// --- Rate limiting methods ---

// CheckRateLimit implements token bucket rate limiting
// Returns true if request is allowed, false if rate limited
func (c *Client) CheckRateLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	fullKey := "ratelimit:" + key

	// Increment counter
	count, err := c.client.Incr(ctx, fullKey).Result()
	if err != nil {
		return false, err
	}

	// Set expiration on first request
	if count == 1 {
		if err := c.client.Expire(ctx, fullKey, window).Err(); err != nil {
			return false, err
		}
	}

	return count <= int64(limit), nil
}

// GetRateLimitRemaining returns remaining requests in window
func (c *Client) GetRateLimitRemaining(ctx context.Context, key string, limit int) (int, error) {
	fullKey := "ratelimit:" + key
	count, err := c.client.Get(ctx, fullKey).Int()
	if err == redis.Nil {
		return limit, nil
	}
	if err != nil {
		return 0, err
	}

	remaining := limit - count
	if remaining < 0 {
		return 0, nil
	}
	return remaining, nil
}

// ResetRateLimit clears rate limit counter
func (c *Client) ResetRateLimit(ctx context.Context, key string) error {
	return c.Delete(ctx, "ratelimit:"+key)
}

// --- Queue methods (for distributed job queue) ---

// EnqueueJob adds a job to a Redis list (queue)
func (c *Client) EnqueueJob(ctx context.Context, queue string, jobData interface{}) error {
	data, err := json.Marshal(jobData)
	if err != nil {
		return err
	}
	return c.client.RPush(ctx, "queue:"+queue, data).Err()
}

// DequeueJob removes and returns a job from queue (blocking)
func (c *Client) DequeueJob(ctx context.Context, queue string, timeout time.Duration) (string, error) {
	result, err := c.client.BLPop(ctx, timeout, "queue:"+queue).Result()
	if err != nil {
		return "", err
	}
	if len(result) < 2 {
		return "", fmt.Errorf("invalid queue response")
	}
	return result[1], nil
}

// GetQueueLength returns number of jobs in queue
func (c *Client) GetQueueLength(ctx context.Context, queue string) (int64, error) {
	return c.client.LLen(ctx, "queue:"+queue).Result()
}

// --- Distributed lock methods ---

// AcquireLock tries to acquire a distributed lock
func (c *Client) AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return c.client.SetNX(ctx, "lock:"+key, "1", ttl).Result()
}

// ReleaseLock releases a distributed lock
func (c *Client) ReleaseLock(ctx context.Context, key string) error {
	return c.Delete(ctx, "lock:"+key)
}

// --- Utility methods ---

// Ping checks Redis connection
func (c *Client) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// FlushDB clears all keys in current database (use with caution!)
func (c *Client) FlushDB(ctx context.Context) error {
	return c.client.FlushDB(ctx).Err()
}
