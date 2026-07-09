// Package jsonx 提供 JSON 相关的通用工具，当前包含敏感字段递归脱敏能力。
// 契约要求:任意返回原始 JSON(工具参数/结果、AgentState、事件 input/output、系统设置等)
// 的接口，都必须对 key 命中敏感词的值做脱敏，避免 token/secret/password 等信息外泄。
package jsonx

import (
	"encoding/json"
	"strings"
)

// sensitiveKeywords 命中即脱敏的 key 关键词，大小写不敏感、允许子串匹配。
var sensitiveKeywords = []string{
	"token",
	"secret",
	"password",
	"api_key",
	"apikey",
	"authorization",
}

const redactedPlaceholder = "***"

// IsSensitiveKey 判断字段名是否命中敏感关键词(大小写不敏感、子串匹配)。
func IsSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, keyword := range sensitiveKeywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}

// RedactJSON 对任意 JSON 原始字节做递归脱敏，key 命中敏感词的值统一替换为 "***"。
// 输入非法 JSON 或空值时原样返回，调用方不需要额外做 nil/空值判断。
func RedactJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		// 不是合法 JSON(例如已经是字符串摘要)时不处理，避免破坏原始内容。
		return raw
	}
	redacted := redactValue(value)
	out, err := json.Marshal(redacted)
	if err != nil {
		return raw
	}
	return json.RawMessage(out)
}

// RedactValue 对任意已解析的 Go 值(map/slice/基础类型)做递归脱敏，返回脱敏后的新值。
func RedactValue(value any) any {
	return redactValue(value)
}

// redactValue 是脱敏的核心递归实现。
func redactValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, v := range typed {
			if IsSensitiveKey(key) {
				result[key] = redactedPlaceholder
				continue
			}
			result[key] = redactValue(v)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for i, v := range typed {
			result[i] = redactValue(v)
		}
		return result
	default:
		return typed
	}
}
