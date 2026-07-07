package ids

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// New 生成带业务前缀、时间戳和随机熵的可读 ID。
func New(prefix string) (string, error) {
	prefix = strings.Trim(strings.ToLower(strings.TrimSpace(prefix)), "_-")
	if prefix == "" {
		return "", fmt.Errorf("id prefix 不能为空")
	}

	// 随机熵放在时间戳之后，既方便按生成时间粗略排查，也降低并发冲突概率。
	var entropy [10]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", fmt.Errorf("生成随机 ID 失败: %w", err)
	}

	timestamp := strconv.FormatInt(time.Now().UTC().UnixMilli(), 36)
	randomPart := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(entropy[:])
	return prefix + "_" + timestamp + "_" + strings.ToLower(randomPart), nil
}
