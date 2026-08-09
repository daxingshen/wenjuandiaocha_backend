// single-choice / multi-choice 的 engine 侧行为。复刻 packages/question-types/src/{single-choice,multi-choice}/handler.ts。
package qtype

import (
	"fmt"

	"wenjuandiaocha_backend/internal/domain"
)

// ---------- single-choice ----------
// answer 形状:string(选项 value)。

type singleChoiceProps struct {
	Options []option `json:"options"`
}

type singleChoice struct{}

func (singleChoice) Type() string { return "single-choice" }

func (singleChoice) Validate(q domain.Question, answer any) string {
	var p singleChoiceProps
	unmarshalProps(q.Props, &p)
	s, ok := answer.(string)
	if !ok {
		return "答案格式应为单个选项"
	}
	for _, o := range p.Options {
		if o.Value == s {
			return ""
		}
	}
	return "所选选项不存在"
}

func (singleChoice) Normalize(q domain.Question, answer any) []domain.NormalizedRow {
	s, ok := answer.(string)
	if !ok || s == "" {
		return nil
	}
	return []domain.NormalizedRow{{QID: q.ID, Value: s}}
}

// ---------- multi-choice ----------
// answer 形状:[]any(选中的选项 value 数组,元素是 string)。
// 关键:required(空数组)判断在此 handler 内,不在通用 ValidateSurvey(02-facts §6 陷阱)。

type multiChoiceProps struct {
	Options []option `json:"options"`
	Min     *int     `json:"min"`
	Max     *int     `json:"max"`
}

type multiChoice struct{}

func (multiChoice) Type() string { return "multi-choice" }

func (multiChoice) Validate(q domain.Question, answer any) string {
	var p multiChoiceProps
	unmarshalProps(q.Props, &p)

	arr, ok := answer.([]any)
	if !ok {
		return "答案格式应为选项数组"
	}
	// 必答:空数组在通用层被当「已答」,故必答空判断落在此处。
	if q.Required && len(arr) == 0 {
		return "此题为必答"
	}
	valid := map[string]bool{}
	for _, o := range p.Options {
		valid[o.Value] = true
	}
	seen := map[string]bool{}
	for _, v := range arr {
		s, isStr := v.(string)
		if !isStr || !valid[s] {
			return "包含不存在的选项"
		}
		seen[s] = true
	}
	if len(seen) != len(arr) {
		return "选项不可重复"
	}
	if p.Min != nil && len(arr) < *p.Min {
		return fmt.Sprintf("至少选择 %d 项", *p.Min)
	}
	if p.Max != nil && len(arr) > *p.Max {
		return fmt.Sprintf("最多选择 %d 项", *p.Max)
	}
	return ""
}

func (multiChoice) Normalize(q domain.Question, answer any) []domain.NormalizedRow {
	arr, ok := answer.([]any)
	if !ok {
		return nil
	}
	// 一个选中项一行;交叉分析/频次按 value 聚合。
	var out []domain.NormalizedRow
	for _, v := range arr {
		if s, isStr := v.(string); isStr && s != "" {
			out = append(out, domain.NormalizedRow{QID: q.ID, Value: s})
		}
	}
	return out
}
