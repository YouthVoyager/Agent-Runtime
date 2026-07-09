// Package sqlutil 提供跨用例复用的动态 SQL 过滤条件构造工具。
// 管理台新增的跨任务查询接口(tool-calls、events、artifacts、audit-logs 等)
// 过滤参数组合很多，sqlc 静态查询难以穷举，这里统一用参数化拼接方式构造安全 SQL。
package sqlutil

import (
	"fmt"
	"strings"
)

// Filter 累积 WHERE 条件和对应的位置参数，调用方负责给出正确的列名，
// 值全部通过占位符传递，不做任何字符串拼接，避免注入风险。
type Filter struct {
	conditions []string
	args       []any
}

// New 创建一个空的过滤条件构造器。
func New() *Filter {
	return &Filter{}
}

// Eq 追加等值条件，value 为空字符串时跳过(表示调用方未传该过滤参数)。
func (f *Filter) Eq(column string, value string) *Filter {
	if strings.TrimSpace(value) == "" {
		return f
	}
	f.args = append(f.args, value)
	f.conditions = append(f.conditions, fmt.Sprintf("%s = $%d", column, len(f.args)))
	return f
}

// EqAny 追加任意类型等值条件，value 为 nil 时跳过。
func (f *Filter) EqAny(column string, value any) *Filter {
	if value == nil {
		return f
	}
	f.args = append(f.args, value)
	f.conditions = append(f.conditions, fmt.Sprintf("%s = $%d", column, len(f.args)))
	return f
}

// Gte 追加大于等于条件，value 为 nil 时跳过。
func (f *Filter) Gte(column string, value any) *Filter {
	if value == nil {
		return f
	}
	f.args = append(f.args, value)
	f.conditions = append(f.conditions, fmt.Sprintf("%s >= $%d", column, len(f.args)))
	return f
}

// Lte 追加小于等于条件，value 为 nil 时跳过。
func (f *Filter) Lte(column string, value any) *Filter {
	if value == nil {
		return f
	}
	f.args = append(f.args, value)
	f.conditions = append(f.conditions, fmt.Sprintf("%s <= $%d", column, len(f.args)))
	return f
}

// Like 追加模糊匹配条件，value 为空字符串时跳过。
func (f *Filter) Like(column string, value string) *Filter {
	if strings.TrimSpace(value) == "" {
		return f
	}
	f.args = append(f.args, "%"+value+"%")
	f.conditions = append(f.conditions, fmt.Sprintf("%s ilike $%d", column, len(f.args)))
	return f
}

// Raw 追加自定义条件表达式，占位符使用 ? 会被自动替换为按顺序编号的 $N。
func (f *Filter) Raw(exprWithQuestionMarks string, values ...any) *Filter {
	if len(values) == 0 {
		return f
	}
	expr := exprWithQuestionMarks
	for _, v := range values {
		f.args = append(f.args, v)
		expr = strings.Replace(expr, "?", fmt.Sprintf("$%d", len(f.args)), 1)
	}
	f.conditions = append(f.conditions, expr)
	return f
}

// Where 返回拼接好的 WHERE 子句(包含前导 "where" 关键字，无条件时返回空字符串)。
func (f *Filter) Where() string {
	if len(f.conditions) == 0 {
		return ""
	}
	return "where " + strings.Join(f.conditions, " and ")
}

// Args 返回按追加顺序排列的参数列表。
func (f *Filter) Args() []any {
	return f.args
}

// NextPlaceholder 返回下一个可用占位符序号，供调用方拼接 LIMIT/OFFSET 等追加参数。
func (f *Filter) NextPlaceholder() int {
	return len(f.args) + 1
}

// AddArg 追加一个不参与 WHERE 条件的参数(例如 LIMIT/OFFSET)，返回其占位符编号。
func (f *Filter) AddArg(value any) int {
	f.args = append(f.args, value)
	return len(f.args)
}
