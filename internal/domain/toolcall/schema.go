package toolcall

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ParamSpec 描述单个工具参数的约束,遵循 JSON Schema 关键字子集。
type ParamSpec struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	MaxLength   int    `json:"maxLength,omitempty"`
}

// ArgumentSchema 是工具参数的 JSON Schema(子集):object + properties + required + additionalProperties。
type ArgumentSchema struct {
	Type                 string               `json:"type"`
	Properties           map[string]ParamSpec `json:"properties"`
	Required             []string             `json:"required,omitempty"`
	AdditionalProperties bool                 `json:"additionalProperties"`
}

// ValidateArguments 按工具 schema 校验规范化后的参数,失败返回带字段名的错误。
func ValidateArguments(schema ArgumentSchema, raw json.RawMessage) error {
	var args map[string]any
	if err := json.Unmarshal(raw, &args); err != nil {
		return fmt.Errorf("arguments 必须是 JSON 对象: %w", err)
	}
	for _, field := range schema.Required {
		value, ok := args[field]
		if !ok {
			return fmt.Errorf("缺少必填参数 %q", field)
		}
		if text, isString := value.(string); isString && strings.TrimSpace(text) == "" {
			return fmt.Errorf("必填参数 %q 不能为空", field)
		}
	}
	// 按字段名排序校验,保证错误信息确定性。
	keys := make([]string, 0, len(args))
	for key := range args {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		spec, known := schema.Properties[key]
		if !known {
			if schema.AdditionalProperties {
				continue
			}
			return fmt.Errorf("未定义的参数 %q", key)
		}
		if err := validateParamType(key, spec, args[key]); err != nil {
			return err
		}
	}
	return nil
}

// validateParamType 校验参数值与 schema 声明类型一致。
func validateParamType(field string, spec ParamSpec, value any) error {
	switch spec.Type {
	case "string":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("参数 %q 必须是 string", field)
		}
		if spec.MaxLength > 0 && len(text) > spec.MaxLength {
			return fmt.Errorf("参数 %q 超过最大长度 %d", field, spec.MaxLength)
		}
	case "number":
		if _, ok := value.(float64); !ok {
			return fmt.Errorf("参数 %q 必须是 number", field)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("参数 %q 必须是 boolean", field)
		}
	case "object":
		if _, ok := value.(map[string]any); !ok {
			return fmt.Errorf("参数 %q 必须是 object", field)
		}
	case "array":
		if _, ok := value.([]any); !ok {
			return fmt.Errorf("参数 %q 必须是 array", field)
		}
	default:
		return fmt.Errorf("参数 %q 的 schema 类型 %q 不支持", field, spec.Type)
	}
	return nil
}
