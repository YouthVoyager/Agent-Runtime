package loopdetect

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// DefaultThreshold 定义连续相同 step 指纹达到多少次判定为循环。
const DefaultThreshold = 3

// MaxHistory 限制 agent_states.loop_fingerprints 保留的历史长度。
const MaxHistory = 50

type Result struct {
	Detected    bool   `json:"detected"`
	Fingerprint string `json:"fingerprint"`
	Repeats     int    `json:"repeats"`
	Threshold   int    `json:"threshold"`
}

// BuildFingerprint 根据 step 的工具意图和模型输出生成稳定指纹。
func BuildFingerprint(toolName string, argumentsHash string, content string) string {
	raw := fmt.Sprintf("%s|%s|%s",
		strings.ToLower(strings.TrimSpace(toolName)),
		strings.TrimSpace(argumentsHash),
		strings.TrimSpace(content),
	)
	sum := sha256.Sum256([]byte(raw))
	return "fp_" + hex.EncodeToString(sum[:16])
}

// Detect 判断当前指纹加入历史后是否构成循环:尾部连续相同指纹次数达到阈值。
func Detect(history []string, fingerprint string, threshold int) Result {
	if threshold <= 0 {
		threshold = DefaultThreshold
	}
	repeats := 1
	for i := len(history) - 1; i >= 0; i-- {
		if history[i] != fingerprint {
			break
		}
		repeats++
	}
	return Result{
		Detected:    repeats >= threshold,
		Fingerprint: fingerprint,
		Repeats:     repeats,
		Threshold:   threshold,
	}
}

// Append 将指纹追加到历史并裁剪到 MaxHistory。
func Append(history []string, fingerprint string) []string {
	history = append(history, fingerprint)
	if len(history) > MaxHistory {
		history = history[len(history)-MaxHistory:]
	}
	return history
}
