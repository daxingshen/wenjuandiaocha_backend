// text-input(单项填空)/ textarea(多行文本)的 engine 侧行为。
// 复刻 packages/question-types/src/{text-input,textarea}/handler.ts。answer 形状:string。
//
// maxLength 语义:前端用 JS `answer.length`(UTF-16 code unit 数)。这里用 utf16.Encode
// 逐位对齐,避免中文/emoji 场景与前端体验校验漂移(见 02-facts §6)。
package qtype

import (
	"regexp"
	"unicode/utf16"

	"wenjuandiaocha_backend/internal/domain"
)

// utf16Len 复刻 JS string.length(UTF-16 code unit 计数)。
func utf16Len(s string) int {
	return len(utf16.Encode([]rune(s)))
}

// ---------- text-input ----------

// 邮箱/手机正则,与前端 handler.ts 完全一致。
var (
	emailRE = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
	phoneRE = regexp.MustCompile(`^1\d{10}$`)
)

type textInputProps struct {
	Format    string `json:"format"`    // 11 项属性验证;省略等于 text(见 textformat.go)
	MinLength *int   `json:"minLength"` // 省略不限
	MaxLength *int   `json:"maxLength"` // 省略不限
	// DefaultValue 作答态初始回显,仅供前端;后端不校验(与 dropdown 先例一致)。
	DefaultValue *string `json:"defaultValue"`
}

type textInput struct{}

func (textInput) Type() string { return "text-input" }

func (textInput) Validate(q domain.Question, answer any) string {
	var p textInputProps
	unmarshalProps(q.Props, &p)
	return validateTextValue(answer, textRules{Format: p.Format, MinLength: p.MinLength, MaxLength: p.MaxLength})
}

func (textInput) Normalize(q domain.Question, answer any) []domain.NormalizedRow {
	s, ok := answer.(string)
	if !ok || s == "" {
		return nil
	}
	return []domain.NormalizedRow{{QID: q.ID, Value: s}}
}

// ---------- textarea ----------

type textareaProps struct {
	MinLength    *int    `json:"minLength"` // 省略不限
	MaxLength    *int    `json:"maxLength"` // 省略不限
	DefaultValue *string `json:"defaultValue"` // 前端回显,后端不校验
}

type textarea struct{}

func (textarea) Type() string { return "textarea" }

func (textarea) Validate(q domain.Question, answer any) string {
	var p textareaProps
	unmarshalProps(q.Props, &p)
	// 多行文本无属性验证(format 恒 text),只校长度。
	return validateTextValue(answer, textRules{Format: "text", MinLength: p.MinLength, MaxLength: p.MaxLength})
}

func (textarea) Normalize(q domain.Question, answer any) []domain.NormalizedRow {
	s, ok := answer.(string)
	if !ok || s == "" {
		return nil
	}
	return []domain.NormalizedRow{{QID: q.ID, Value: s}}
}
