package accounts

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type redisAccountStorage struct {
	client *redis.Client
	prefix string
}

func newRedisAccountStorage(addr, password string, db int, prefix string) (accountStorageBackend, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     strings.TrimSpace(addr),
		Password: password,
		DB:       db,
	})
	return &redisAccountStorage{
		client: client,
		prefix: firstNonEmpty(prefix, "chatgpt2api:studio"),
	}, nil
}

func (s *redisAccountStorage) Init() error {
	return s.client.Ping(context.Background()).Err()
}

func (s *redisAccountStorage) Close() error {
	if s.client == nil {
		return nil
	}
	return s.client.Close()
}

func (s *redisAccountStorage) LoadAuths() ([]LocalAuth, error) {
	values, err := s.client.HGetAll(context.Background(), s.key("auths")).Result()
	if err != nil {
		return nil, err
	}
	result := make([]LocalAuth, 0, len(values))
	for name, raw := range values {
		auth, err := parseLocalAuth(name, name, []byte(raw), "")
		if err != nil {
			continue
		}
		result = append(result, auth)
	}
	return result, nil
}

func (s *redisAccountStorage) ReadAuthRaw(name string) ([]byte, error) {
	value, err := s.client.HGet(context.Background(), s.key("auths"), filepathBase(name)).Result()
	if err != nil {
		return nil, err
	}
	return []byte(value), nil
}

func (s *redisAccountStorage) SaveAuthRaw(name string, raw []byte) error {
	return s.client.HSet(context.Background(), s.key("auths"), filepathBase(name), raw).Err()
}

func (s *redisAccountStorage) DeleteAuth(name string) error {
	return s.client.HDel(context.Background(), s.key("auths"), filepathBase(name)).Err()
}

func (s *redisAccountStorage) LoadRuntimeStates() (map[string]RuntimeState, error) {
	raw, err := s.client.Get(context.Background(), s.key("runtime_state")).Bytes()
	if err == redis.Nil {
		return map[string]RuntimeState{}, nil
	}
	if err != nil {
		return nil, err
	}
	var result map[string]RuntimeState
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	if result == nil {
		return map[string]RuntimeState{}, nil
	}
	return result, nil
}

func (s *redisAccountStorage) SaveRuntimeStates(states map[string]RuntimeState) error {
	raw, err := json.Marshal(states)
	if err != nil {
		return err
	}
	return s.client.Set(context.Background(), s.key("runtime_state"), raw, 0).Err()
}

func (s *redisAccountStorage) LoadSyncStates() (map[string]SyncState, error) {
	values, err := s.client.HGetAll(context.Background(), s.key("sync_states")).Result()
	if err != nil {
		return nil, err
	}
	result := map[string]SyncState{}
	for name, raw := range values {
		var state SyncState
		if err := json.Unmarshal([]byte(raw), &state); err != nil {
			continue
		}
		if state.Name == "" {
			state.Name = name
		}
		result[state.Name] = state
	}
	return result, nil
}

func (s *redisAccountStorage) SaveSyncState(state SyncState) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return s.client.HSet(context.Background(), s.key("sync_states"), state.Name, raw).Err()
}

func (s *redisAccountStorage) DeleteSyncState(name string) error {
	return s.client.HDel(context.Background(), s.key("sync_states"), name).Err()
}

func (s *redisAccountStorage) EnsureSyncStateInitialized(auths []LocalAuth) error {
	ctx := context.Background()
	initialized, err := s.client.HGet(ctx, s.key("meta"), "sync_bootstrap").Result()
	if err == nil && initialized == "1" {
		return nil
	}
	if err != nil && err != redis.Nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, auth := range auths {
		exists, err := s.client.HExists(ctx, s.key("sync_states"), auth.Name).Result()
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		raw, err := json.Marshal(SyncState{Name: auth.Name, Origin: "local", LastSyncedAt: now})
		if err != nil {
			return err
		}
		if err := s.client.HSet(ctx, s.key("sync_states"), auth.Name, raw).Err(); err != nil {
			return err
		}
	}
	return s.client.HSet(ctx, s.key("meta"), "sync_bootstrap", "1").Err()
}

func (s *redisAccountStorage) LoadImageRoutingPolicy() (*ImageAccountRoutingPolicy, error) {
	raw, err := s.client.HGet(context.Background(), s.key("meta"), "image_account_policy").Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var policy ImageAccountRoutingPolicy
	if err := json.Unmarshal([]byte(raw), &policy); err != nil {
		return nil, err
	}
	return &policy, nil
}

func (s *redisAccountStorage) SaveImageRoutingPolicy(policy ImageAccountRoutingPolicy) error {
	raw, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	return s.client.HSet(context.Background(), s.key("meta"), "image_account_policy", raw).Err()
}

func (s *redisAccountStorage) key(name string) string {
	return s.prefix + ":" + name
}

func filepathBase(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(strings.ReplaceAll(trimmed, "\\", "/"), "/")
	return parts[len(parts)-1]
}
