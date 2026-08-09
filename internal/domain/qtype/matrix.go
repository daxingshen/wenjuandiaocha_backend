// matrix-single(矩阵单选)的 engine 侧行为。复刻 packages/question-types/src/matrix-single/handler.ts。
// answer 形状:map[string]any(子行 id → 所选列 value,元素是 string)。非对象视作空。
// 关键:required(每子行都要作答)判断在此 handler 内(02-facts §6 陷阱);
// normalize 按 props.rows 顺序产行(不是答案 map 的遍历顺序)。
package qtype

import "wenjuandiaocha_backend/internal/domain"

type matrixSingleProps struct {
	Rows    []row    `json:"rows"`
	Options []option `json:"options"`
}

type matrixSingle struct{}

func (matrixSingle) Type() string { return "matrix-single" }

// readMatrixAnswer 把未知答案安全读成 map[subId]value;非对象一律空。
func readMatrixAnswer(answer any) map[string]any {
	m, ok := answer.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return m
}

// answered 复刻:值非 nil/非空串即已答。
func matrixAnswered(v any) bool {
	if v == nil {
		return false
	}
	s, ok := v.(string)
	if ok && s == "" {
		return false
	}
	return true
}

func (matrixSingle) Validate(q domain.Question, answer any) string {
	var p matrixSingleProps
	unmarshalProps(q.Props, &p)
	ans := readMatrixAnswer(answer)

	validValues := map[string]bool{}
	for _, o := range p.Options {
		validValues[o.Value] = true
	}
	rowIDs := map[string]bool{}
	for _, r := range p.Rows {
		rowIDs[r.ID] = true
	}

	for subID, val := range ans {
		if !matrixAnswered(val) {
			continue
		}
		if !rowIDs[subID] {
			return "存在不属于本题的子项"
		}
		s, ok := val.(string)
		if !ok || !validValues[s] {
			return "所选选项不存在"
		}
	}
	// 必答:每个子行都要有合法作答。
	if q.Required {
		for _, r := range p.Rows {
			if !matrixAnswered(ans[r.ID]) {
				return "每个子项都需作答"
			}
		}
	}
	return ""
}

func (matrixSingle) Normalize(q domain.Question, answer any) []domain.NormalizedRow {
	var p matrixSingleProps
	unmarshalProps(q.Props, &p)
	ans := readMatrixAnswer(answer)

	// 按 props.rows 顺序遍历,每个已答子行一行 { qid, subId, value }。
	var out []domain.NormalizedRow
	for _, r := range p.Rows {
		v := ans[r.ID]
		if !matrixAnswered(v) {
			continue
		}
		s, ok := v.(string)
		if !ok {
			continue
		}
		out = append(out, domain.NormalizedRow{QID: q.ID, SubID: r.ID, Value: s})
	}
	return out
}
