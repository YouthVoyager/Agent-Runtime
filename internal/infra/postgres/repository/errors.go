package repository

import (
	"errors"

	"github.com/jackc/pgx/v5"
)

var (
	ErrNotFound        = errors.New("repository: 记录不存在")
	ErrVersionConflict = errors.New("repository: 状态版本冲突")
)

// mapNotFound 将 pgx 无行错误转换为仓储层统一的不存在错误。
func mapNotFound(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// mapVersionConflict 将乐观锁更新未命中的场景转换为版本冲突错误。
func mapVersionConflict(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrVersionConflict
	}
	return err
}
