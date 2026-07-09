package jsonx

import (
	"encoding/json"
	"testing"
)

// TestRedactJSON 校验递归脱敏能处理嵌套 object、array 和大小写/子串命中的 key。
func TestRedactJSON(t *testing.T) {
	input := json.RawMessage(`{
		"api_key": "sk-123456",
		"Authorization": "Bearer xxx",
		"nested": {"user_password": "p@ss", "normal": "keep-me"},
		"list": [{"secretValue": "s1"}, {"safe": "s2"}],
		"count": 3,
		"flag": true
	}`)

	redacted := RedactJSON(input)

	var got map[string]any
	if err := json.Unmarshal(redacted, &got); err != nil {
		t.Fatalf("脱敏结果不是合法 JSON: %v", err)
	}
	if got["api_key"] != "***" {
		t.Fatalf("api_key 未脱敏: %v", got["api_key"])
	}
	if got["Authorization"] != "***" {
		t.Fatalf("Authorization 未脱敏: %v", got["Authorization"])
	}
	nested, ok := got["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested 字段类型不正确: %T", got["nested"])
	}
	if nested["user_password"] != "***" {
		t.Fatalf("嵌套 password 未脱敏: %v", nested["user_password"])
	}
	if nested["normal"] != "keep-me" {
		t.Fatalf("非敏感字段被误脱敏: %v", nested["normal"])
	}
	list, ok := got["list"].([]any)
	if !ok || len(list) != 2 {
		t.Fatalf("list 字段类型或长度不正确: %v", got["list"])
	}
	first, _ := list[0].(map[string]any)
	if first["secretValue"] != "***" {
		t.Fatalf("数组内敏感字段未脱敏: %v", first["secretValue"])
	}
	second, _ := list[1].(map[string]any)
	if second["safe"] != "s2" {
		t.Fatalf("数组内非敏感字段被误脱敏: %v", second["safe"])
	}
	if got["count"] != float64(3) || got["flag"] != true {
		t.Fatalf("基础类型字段被破坏: count=%v flag=%v", got["count"], got["flag"])
	}
}

// TestRedactJSONEmptyOrInvalid 校验空值和非法 JSON 原样返回，不会 panic。
func TestRedactJSONEmptyOrInvalid(t *testing.T) {
	if got := RedactJSON(nil); got != nil {
		t.Fatalf("nil 输入应原样返回: %v", got)
	}
	invalid := json.RawMessage(`not-json`)
	if got := RedactJSON(invalid); string(got) != string(invalid) {
		t.Fatalf("非法 JSON 应原样返回: %s", got)
	}
}

// TestIsSensitiveKey 校验敏感 key 判定大小写不敏感且允许子串匹配。
func TestIsSensitiveKey(t *testing.T) {
	cases := map[string]bool{
		"token":         true,
		"AccessToken":   true,
		"api_key":       true,
		"apiKey":        true,
		"SECRET":        true,
		"user_password": true,
		"Authorization": true,
		"goal":          false,
		"status":        false,
	}
	for key, want := range cases {
		if got := IsSensitiveKey(key); got != want {
			t.Fatalf("IsSensitiveKey(%q) = %v want %v", key, got, want)
		}
	}
}
