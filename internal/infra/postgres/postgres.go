package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PoolConfig struct {
	DatabaseURL     string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

// NewPool 创建并验证 PostgreSQL 连接池。
func NewPool(ctx context.Context, config PoolConfig) (*pgxpool.Pool, error) {
	if config.DatabaseURL == "" {
		return nil, fmt.Errorf("database url 不能为空")
	}

	poolConfig, err := pgxpool.ParseConfig(config.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("解析 PostgreSQL 配置失败: %w", err)
	}

	if config.MaxConns > 0 {
		poolConfig.MaxConns = config.MaxConns
	}
	if config.MinConns > 0 {
		poolConfig.MinConns = config.MinConns
	}
	if config.MaxConnLifetime > 0 {
		poolConfig.MaxConnLifetime = config.MaxConnLifetime
	}
	if config.MaxConnIdleTime > 0 {
		poolConfig.MaxConnIdleTime = config.MaxConnIdleTime
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("创建 PostgreSQL 连接池失败: %w", err)
	}
	// 启动阶段主动 ping，避免服务已监听但数据库不可用时才在首个请求暴露问题。
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("连接 PostgreSQL 失败: %w", err)
	}
	return pool, nil
}
