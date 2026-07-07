package ids

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func New(prefix string) (string, error) {
	prefix = strings.Trim(strings.ToLower(strings.TrimSpace(prefix)), "_-")
	if prefix == "" {
		return "", fmt.Errorf("id prefix 不能为空")
	}

	var entropy [10]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", fmt.Errorf("生成随机 ID 失败: %w", err)
	}

	timestamp := strconv.FormatInt(time.Now().UTC().UnixMilli(), 36)
	randomPart := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(entropy[:])
	return prefix + "_" + timestamp + "_" + strings.ToLower(randomPart), nil
}
