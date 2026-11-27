package valkey

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	vk "github.com/valkey-io/valkey-go"
)

// Client wraps a Valkey connection for ephemeral runtime state, caches, and atomic single-use tickets.
type Client struct {
	raw vk.Client
}

// Connect opens a Valkey client and verifies connectivity with PING.
func Connect(ctx context.Context, addr, password string) (*Client, error) {
	if addr == "" {
		return nil, errors.New("valkey address is empty")
	}
	client, err := vk.NewClient(vk.ClientOption{
		InitAddress:  []string{addr},
		Password:     password,
		DisableCache: true,
	})
	if err != nil {
		return nil, fmt.Errorf("init valkey client: %w", err)
	}

	c := &Client{raw: client}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := c.Ping(pingCtx); err != nil {
		client.Close()
		return nil, fmt.Errorf("ping valkey %s: %w", addr, err)
	}
	return c, nil
}

// Ping checks Valkey responsiveness (used by /readyz).
func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.raw == nil {
		return nil
	}
	return c.raw.Do(ctx, c.raw.B().Ping().Build()).Error()
}

// Close releases underlying Valkey connections.
func (c *Client) Close() {
	if c != nil && c.raw != nil {
		c.raw.Close()
	}
}

// SetJSON marshals val as JSON and writes it to key with an optional TTL.
func (c *Client) SetJSON(ctx context.Context, key string, val any, ttl time.Duration) error {
	if c == nil || c.raw == nil {
		return nil
	}
	b, err := json.Marshal(val)
	if err != nil {
		return err
	}
	if ttl > 0 {
		return c.raw.Do(ctx, c.raw.B().Set().Key(key).Value(string(b)).Px(ttl).Build()).Error()
	}
	return c.raw.Do(ctx, c.raw.B().Set().Key(key).Value(string(b)).Build()).Error()
}

// GetJSON reads and unmarshals a JSON value from key. Returns (zero, false, nil) on cache miss.
func GetJSON[T any](ctx context.Context, c *Client, key string) (T, bool, error) {
	var zero T
	if c == nil || c.raw == nil {
		return zero, false, nil
	}
	raw, err := c.raw.Do(ctx, c.raw.B().Get().Key(key).Build()).AsBytes()
	if err != nil {
		if vk.IsValkeyNil(err) {
			return zero, false, nil
		}
		return zero, false, err
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		return zero, false, err
	}
	return out, true, nil
}

// ConsumeJSON atomically fetches and deletes key (GETDEL) for single-use WebSocket tickets.
func ConsumeJSON[T any](ctx context.Context, c *Client, key string) (T, bool, error) {
	var zero T
	if c == nil || c.raw == nil {
		return zero, false, nil
	}
	raw, err := c.raw.Do(ctx, c.raw.B().Getdel().Key(key).Build()).AsBytes()
	if err != nil {
		if vk.IsValkeyNil(err) {
			return zero, false, nil
		}
		return zero, false, err
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		return zero, false, err
	}
	return out, true, nil
}

// MGetJSON fetches multiple JSON keys in a single round-trip, returning a map keyed by key name for hits.
func MGetJSON[T any](ctx context.Context, c *Client, keys []string) (map[string]T, error) {
	out := make(map[string]T)
	if c == nil || c.raw == nil || len(keys) == 0 {
		return out, nil
	}
	arr, err := c.raw.Do(ctx, c.raw.B().Mget().Key(keys...).Build()).ToArray()
	if err != nil {
		return nil, err
	}
	for i, msg := range arr {
		if i >= len(keys) || msg.IsNil() {
			continue
		}
		raw, err := msg.AsBytes()
		if err != nil {
			continue
		}
		var item T
		if err := json.Unmarshal(raw, &item); err == nil {
			out[keys[i]] = item
		}
	}
	return out, nil
}

// Del removes one or more keys via UNLINK.
func (c *Client) Del(ctx context.Context, keys ...string) error {
	if c == nil || c.raw == nil || len(keys) == 0 {
		return nil
	}
	return c.raw.Do(ctx, c.raw.B().Unlink().Key(keys...).Build()).Error()
}

// DelByPrefix scans keys matching prefix+"*" and unlinks them in batches.
func (c *Client) DelByPrefix(ctx context.Context, prefix string) error {
	if c == nil || c.raw == nil || prefix == "" {
		return nil
	}
	pattern := prefix + "*"
	var cursor uint64
	for {
		entry, err := c.raw.Do(ctx, c.raw.B().Scan().Cursor(cursor).Match(pattern).Count(200).Build()).AsScanEntry()
		if err != nil {
			return err
		}
		if len(entry.Elements) > 0 {
			if err := c.raw.Do(ctx, c.raw.B().Unlink().Key(entry.Elements...).Build()).Error(); err != nil {
				return err
			}
		}
		cursor = entry.Cursor
		if cursor == 0 {
			return nil
		}
	}
}

// HSetJSON writes a single field as JSON into hash key and optionally refreshes the hash TTL.
func (c *Client) HSetJSON(ctx context.Context, key, field string, val any, ttl time.Duration) error {
	if c == nil || c.raw == nil {
		return nil
	}
	b, err := json.Marshal(val)
	if err != nil {
		return err
	}
	if err := c.raw.Do(ctx, c.raw.B().Hset().Key(key).FieldValue().FieldValue(field, string(b)).Build()).Error(); err != nil {
		return err
	}
	if ttl > 0 {
		return c.raw.Do(ctx, c.raw.B().Pexpire().Key(key).Milliseconds(ttl.Milliseconds()).Build()).Error()
	}
	return nil
}

// HSetManyJSON writes multiple fields as JSON into hash key in one HSET command and refreshes TTL.
func (c *Client) HSetManyJSON(ctx context.Context, key string, fields map[string]any, ttl time.Duration) error {
	if c == nil || c.raw == nil || len(fields) == 0 {
		return nil
	}
	fv := c.raw.B().Hset().Key(key).FieldValue()
	for k, v := range fields {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		fv = fv.FieldValue(k, string(b))
	}
	if err := c.raw.Do(ctx, fv.Build()).Error(); err != nil {
		return err
	}
	if ttl > 0 {
		return c.raw.Do(ctx, c.raw.B().Pexpire().Key(key).Milliseconds(ttl.Milliseconds()).Build()).Error()
	}
	return nil
}

// HGetAllJSON reads all fields from hash key and unmarshals each into T.
func HGetAllJSON[T any](ctx context.Context, c *Client, key string) (map[string]T, error) {
	out := make(map[string]T)
	if c == nil || c.raw == nil {
		return out, nil
	}
	strMap, err := c.raw.Do(ctx, c.raw.B().Hgetall().Key(key).Build()).AsStrMap()
	if err != nil {
		if vk.IsValkeyNil(err) {
			return out, nil
		}
		return nil, err
	}
	for field, raw := range strMap {
		var item T
		if err := json.Unmarshal([]byte(raw), &item); err == nil {
			out[field] = item
		}
	}
	return out, nil
}

// HDel deletes one or more fields from a hash key.
func (c *Client) HDel(ctx context.Context, key string, fields ...string) error {
	if c == nil || c.raw == nil || len(fields) == 0 {
		return nil
	}
	return c.raw.Do(ctx, c.raw.B().Hdel().Key(key).Field(fields...).Build()).Error()
}

// HLen returns the number of fields in a hash key (0 if missing or client is nil).
func (c *Client) HLen(ctx context.Context, key string) (int64, error) {
	if c == nil || c.raw == nil {
		return 0, nil
	}
	n, err := c.raw.Do(ctx, c.raw.B().Hlen().Key(key).Build()).AsInt64()
	if err != nil {
		if vk.IsValkeyNil(err) {
			return 0, nil
		}
		return 0, err
	}
	return n, nil
}

// CountByPrefix counts keys matching prefix+"*" using SCAN.
func (c *Client) CountByPrefix(ctx context.Context, prefix string) (int, error) {
	if c == nil || c.raw == nil || prefix == "" {
		return 0, nil
	}
	pattern := prefix + "*"
	var cursor uint64
	total := 0
	for {
		entry, err := c.raw.Do(ctx, c.raw.B().Scan().Cursor(cursor).Match(pattern).Count(200).Build()).AsScanEntry()
		if err != nil {
			return total, err
		}
		total += len(entry.Elements)
		cursor = entry.Cursor
		if cursor == 0 {
			return total, nil
		}
	}
}

// TTLSeconds returns remaining TTL in seconds for key (-1 if persistent, -2 if missing).
func (c *Client) TTLSeconds(ctx context.Context, key string) (int64, error) {
	if c == nil || c.raw == nil {
		return -2, nil
	}
	return c.raw.Do(ctx, c.raw.B().Ttl().Key(key).Build()).AsInt64()
}
