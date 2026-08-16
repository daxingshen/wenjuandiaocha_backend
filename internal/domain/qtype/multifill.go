// multi-fill(多项填空)的 engine 侧行为。复刻前端 packages/question-types/src/multi-fill/handler.ts。
// 「一题干、多个单行框」,每框独立 format/min/maxLength/defaultValue。answer 形状:map[blankId]string。
// normalize 每已答框一行(subId=blankId,按 props.blanks 顺序);必答=每框非空(handler 内判,防空对象绕过)。
package qtype

import "wenjuandiaocha_backend/internal/domain"

// multiFillBlank 一个填空框的配置。DefaultValue 仅前端回显,后端不校验。
type multiFillBlank struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Format    string `json:"format"`
	MinLength *int   `json:"minLength"`
	MaxLength *int   `json:"maxLength"`
}

type multiFillProps struct {
	Blanks []multiFillBlank `json:"blanks"`
}

type multiFill struct{}

func (multiFill) Type() string { return "multi-fill" }

func (multiFill) Validate(q domain.Question, answer any) string {
	var p multiFillProps
	unmarshalProps(q.Props, &p)
	ans := readMatrixAnswer(answer)

	byID := map[string]multiFillBlank{}
	for _, b := range p.Blanks {
		byID[b.ID] = b
	}

	// 逐框校验已答值(空框跳过,由下方必答段统一判)。
	for subID, val := range ans {
		if val == nil {
			continue
		}
		if s, ok := val.(string); ok && s == "" {
			continue
		}
		blank, ok := byID[subID]
		if !ok {
			return "存在不属于本题的填空框"
		}
		if msg := validateTextValue(val, textRules{Format: blank.Format, MinLength: blank.MinLength, MaxLength: blank.MaxLength}); msg != "" {
			return msg
		}
	}
	// 必答:每框都要非空(空对象在通用层算已答,故在此判)。
	if q.Required {
		for _, b := range p.Blanks {
			if !strAnswered(ans[b.ID]) {
				return "每个填空框都需作答"
			}
		}
	}
	return ""
}

func (multiFill) Normalize(q domain.Question, answer any) []domain.NormalizedRow {
	var p multiFillProps
	unmarshalProps(q.Props, &p)
	ans := readMatrixAnswer(answer)

	var out []domain.NormalizedRow
	for _, b := range p.Blanks {
		s, ok := ans[b.ID].(string)
		if !ok || s == "" {
			continue
		}
		out = append(out, domain.NormalizedRow{QID: q.ID, SubID: b.ID, Value: s})
	}
	return out
}
