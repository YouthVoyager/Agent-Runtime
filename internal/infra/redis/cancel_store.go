package redis

import (
	"context"
	"strings"
	"time"

	redisclient "github.com/redis/go-redis/v9"
)

const cancelFlagTTL = 24 * time.Hour

type CancelStore struct {
	client *redisclient.Client
}

// NewCancelStore 创建任务取消标记存储。
func NewCancelStore(addr string) *CancelStore {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil
	}
	return &CancelStore{client: redisclient.NewClient(&redisclient.Options{Addr: addr})}
}

// Set 写入任务取消标记，Worker 会在安全边界读取该标记。
func (s *CancelStore) Set(ctx context.Context, taskID string) error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Set(ctx, cancelKey(taskID), "1", cancelFlagTTL).Err()
}

// IsCanceled 判断任务是否已请求取消。
func (s *CancelStore) IsCanceled(ctx context.Context, taskID string) bool {
	if s == nil || s.client == nil {
		return false
	}
	value, err := s.client.Get(ctx, cancelKey(taskID)).Result()
	return err == nil && value == "1"
}

// Clear 清理任务取消标记。
func (s *CancelStore) Clear(ctx context.Context, taskID string) error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Del(ctx, cancelKey(taskID)).Err()
}

// Close 关闭 Redis 客户端连接。
func (s *CancelStore) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Close()
}

// cancelKey 返回 Redis 中保存取消标记的 key。
func cancelKey(taskID string) string {
	return "stableagent:cancel:" + strings.TrimSpace(taskID)
}
