// Package objectstore 提供 S3 兼容对象存储(MinIO)客户端,保存大 artifact 内容。
package objectstore

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Store 抽象对象存储,生产可替换为 S3。
type Store interface {
	// Put 上传对象并返回可回读的 storage key。
	Put(ctx context.Context, key string, contentType string, data []byte) error
	// Get 读取对象内容。
	Get(ctx context.Context, key string) ([]byte, error)
}

type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

type minioStore struct {
	client *minio.Client
	bucket string
}

// NewMinIO 创建 MinIO 客户端并确保 bucket 存在;endpoint 为空时返回 nil 表示未启用。
func NewMinIO(ctx context.Context, cfg Config) (Store, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, nil
	}
	if cfg.Bucket == "" {
		cfg.Bucket = "stableagent-artifacts"
	}
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 MinIO 客户端失败: %w", err)
	}
	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("检查 bucket %s 失败: %w", cfg.Bucket, err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{}); err != nil {
			// 并发创建冲突时重新确认存在性。
			if recheck, recheckErr := client.BucketExists(ctx, cfg.Bucket); recheckErr != nil || !recheck {
				return nil, fmt.Errorf("创建 bucket %s 失败: %w", cfg.Bucket, err)
			}
		}
	}
	return &minioStore{client: client, bucket: cfg.Bucket}, nil
}

func (s *minioStore) Put(ctx context.Context, key string, contentType string, data []byte) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	return err
}

func (s *minioStore) Get(ctx context.Context, key string) ([]byte, error) {
	object, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer object.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(object); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
