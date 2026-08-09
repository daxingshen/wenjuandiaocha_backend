// text-input(单项填空)/ textarea(多行文本)的 engine 侧行为。
// 复刻 packages/question-types/src/{text-input,textarea}/handler.ts。answer 形状:string。
//
// maxLength 语义:前端用 JS `answer.length`(UTF-16 code unit 数)。这里用 utf16.Encode
// 逐位对齐,避免中文/emoji 场景与前端体验校验漂移(见 02-facts §6)。
package qtype

import (
	"fmt"
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
	Format    string `json:"format"`    // text|email|phone;省略等于 text
	MaxLength *int   `json:"maxLength"` // 省略不限
}

// readFormat 复刻 readProps:非 email/phone 一律 'text'。
func (p textInputProps) format() string {
	if p.Format == "email" || p.Format == "phone" {
		return p.Format
	}
	return "text"
}

type textInput struct{}

func (textInput) Type() string { return "text-input" }

func (textInput) Validate(q domain.Question, answer any) string {
	var p textInputProps
	unmarshalProps(q.Props, &p)
	s, ok := answer.(string)
	if !ok {
		return "答案格式应为文本"
	}
	if p.MaxLength != nil && utf16Len(s) > *p.MaxLength {
		return fmt.Sprintf("不超过 %d 个字符", *p.MaxLength)
	}
	switch p.format() {
	case "email":
		if !emailRE.MatchString(s) {
			return "邮箱格式不正确"
		}
	case "phone":
		if !phoneRE.MatchString(s) {
			return "手机号格式不正确"
		}
	}
	return ""
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
	MaxLength *int `json:"maxLength"`
}

type textarea struct{}

func (textarea) Type() string { return "textarea" }

func (textarea) Validate(q domain.Question, answer any) string {
	var p textareaProps
	unmarshalProps(q.Props, &p)
	s, ok := answer.(string)
	if !ok {
		return "答案格式应为文本"
	}
	if p.MaxLength != nil && utf16Len(s) > *p.MaxLength {
		return fmt.Sprintf("不超过 %d 个字符", *p.MaxLength)
	}
	return ""
}

func (textarea) Normalize(q domain.Question, answer any) []domain.NormalizedRow {
	s, ok := answer.(string)
	if !ok || s == "" {
		return nil
	}
	return []domain.NormalizedRow{{QID: q.ID, Value: s}}
}
