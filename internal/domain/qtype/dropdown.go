// dropdown(下拉单选)的 engine 侧行为。原生 select 单选,语义同 single-choice 但无填空。
// 复刻前端 packages/question-types/src/dropdown/handler.ts。
// answer 形状:string(选中的选项 value)。空串交给通用必答层处理(与 single-choice 一致)。
package qtype

import "wenjuandiaocha_backend/internal/domain"

type dropdownProps struct {
	Options []option `json:"options"`
	// DefaultValue 是创建时的默认选中项,仅供前端回显,后端不校验。
	DefaultValue *string `json:"defaultValue"`
}

type dropdown struct{}

func (dropdown) Type() string { return "dropdown" }

func (dropdown) Validate(q domain.Question, answer any) string {
	var p dropdownProps
	unmarshalProps(q.Props, &p)

	s, ok := answer.(string)
	if !ok {
		return "答案格式应为单个选项"
	}
	// 空串视作未答,交给通用 ValidateSurvey 的必答层(与 single-choice 同语义)。
	if s == "" {
		return ""
	}
	for _, o := range p.Options {
		if o.Value == s {
			return ""
		}
	}
	return "所选选项不存在"
}

func (dropdown) Normalize(q domain.Question, answer any) []domain.NormalizedRow {
	s, ok := answer.(string)
	if !ok || s == "" {
		return nil
	}
	return []domain.NormalizedRow{{QID: q.ID, Value: s}}
}
