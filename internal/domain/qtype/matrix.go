// 矩阵题型五连的 engine 侧行为。复刻 packages/question-types/src/matrix-*/handler.ts,逐条对齐。
// 共同点:一题干、多子行(row),答案是 map[string]any(子行 id → 值);非对象视作空。
// normalize 按 props.rows 顺序产行(不是答案 map 遍历序),每子行带 subId。
// 必答判断在各 handler 内(空对象在通用层算已答,02-facts §6 陷阱)。
package qtype

import (
	"fmt"

	"wenjuandiaocha_backend/internal/domain"
)

// readMatrixAnswer 把未知答案安全读成 map[subId]value;非对象一律空。choice/matrix 共用。
func readMatrixAnswer(answer any) map[string]any {
	m, ok := answer.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return m
}

// strAnswered 值为非空字符串即已答。
func strAnswered(v any) bool {
	s, ok := v.(string)
	return ok && s != ""
}

// ---------- matrix-single(矩阵单选) ----------
// answer 形状:map[string]any(子行 id → 所选列 value,元素 string)。

type matrixSingleProps struct {
	Rows    []row    `json:"rows"`
	Options []option `json:"options"`
}

type matrixSingle struct{}

func (matrixSingle) Type() string { return "matrix-single" }

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
		if !strAnswered(val) {
			continue
		}
		if !rowIDs[subID] {
			return "存在不属于本题的子项"
		}
		s := val.(string)
		if !validValues[s] {
			return "所选选项不存在"
		}
	}
	if q.Required {
		for _, r := range p.Rows {
			if !strAnswered(ans[r.ID]) {
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

	var out []domain.NormalizedRow
	for _, r := range p.Rows {
		s, ok := ans[r.ID].(string)
		if !ok || s == "" {
			continue
		}
		out = append(out, domain.NormalizedRow{QID: q.ID, SubID: r.ID, Value: s})
	}
	return out
}

// ---------- matrix-multi(矩阵多选) ----------
// answer 形状:map[string]any(子行 id → []any,元素 string)。min/max 落在每一行。

type matrixMultiProps struct {
	Rows    []row    `json:"rows"`
	Options []option `json:"options"`
	Min     *int     `json:"min"`
	Max     *int     `json:"max"`
}

type matrixMulti struct{}

func (matrixMulti) Type() string { return "matrix-multi" }

func (matrixMulti) Validate(q domain.Question, answer any) string {
	var p matrixMultiProps
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
		if val == nil {
			continue
		}
		arr, ok := val.([]any)
		if !ok {
			return "答案格式应为选项数组"
		}
		if len(arr) == 0 {
			continue // 空行:非必答时跳过 min/max
		}
		if !rowIDs[subID] {
			return "存在不属于本题的子项"
		}
		seen := map[string]bool{}
		for _, v := range arr {
			s, isStr := v.(string)
			if !isStr || !validValues[s] {
				return "包含不存在的选项"
			}
			seen[s] = true
		}
		if len(seen) != len(arr) {
			return "选项不可重复"
		}
		if p.Min != nil && len(arr) < *p.Min {
			return fmt.Sprintf("每行至少选择 %d 项", *p.Min)
		}
		if p.Max != nil && len(arr) > *p.Max {
			return fmt.Sprintf("每行最多选择 %d 项", *p.Max)
		}
	}
	// 必答:每行都要有至少一项。
	if q.Required {
		for _, r := range p.Rows {
			arr, ok := ans[r.ID].([]any)
			if !ok || len(arr) == 0 {
				return "每个子项都需作答"
			}
			if p.Min != nil && len(arr) < *p.Min {
				return fmt.Sprintf("每行至少选择 %d 项", *p.Min)
			}
		}
	}
	return ""
}

func (matrixMulti) Normalize(q domain.Question, answer any) []domain.NormalizedRow {
	var p matrixMultiProps
	unmarshalProps(q.Props, &p)
	ans := readMatrixAnswer(answer)

	var out []domain.NormalizedRow
	for _, r := range p.Rows {
		arr, ok := ans[r.ID].([]any)
		if !ok {
			continue
		}
		for _, v := range arr {
			if s, isStr := v.(string); isStr && s != "" {
				out = append(out, domain.NormalizedRow{QID: q.ID, SubID: r.ID, Value: s})
			}
		}
	}
	return out
}

// ---------- matrix-scale(矩阵量表) ----------
// 行为与矩阵单选同构;level 仅供前端编辑器回显,后端不校验。
// 列带 score(分值):normalize 产出分值数值(供均值统计),无分值时回落列 value 字符串。

// scaleOption 量表列:比共用 option 多一个 score(分值)。
type scaleOption struct {
	Value string   `json:"value"`
	Label string   `json:"label"`
	Score *float64 `json:"score"`
}

type matrixScaleProps struct {
	Rows    []row         `json:"rows"`
	Options []scaleOption `json:"options"`
}

type matrixScale struct{}

func (matrixScale) Type() string { return "matrix-scale" }

func (matrixScale) Validate(q domain.Question, answer any) string {
	var p matrixScaleProps
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
		if !strAnswered(val) {
			continue
		}
		if !rowIDs[subID] {
			return "存在不属于本题的子项"
		}
		if !validValues[val.(string)] {
			return "所选选项不存在"
		}
	}
	if q.Required {
		for _, r := range p.Rows {
			if !strAnswered(ans[r.ID]) {
				return "每个子项都需作答"
			}
		}
	}
	return ""
}

func (matrixScale) Normalize(q domain.Question, answer any) []domain.NormalizedRow {
	var p matrixScaleProps
	unmarshalProps(q.Props, &p)
	ans := readMatrixAnswer(answer)

	// 列 value → 分值;每子行一行,value 存分值(float64),无分值回落列 value 字符串。
	scoreOf := map[string]*float64{}
	for _, o := range p.Options {
		scoreOf[o.Value] = o.Score
	}

	var out []domain.NormalizedRow
	for _, r := range p.Rows {
		s, ok := ans[r.ID].(string)
		if !ok || s == "" {
			continue
		}
		if score := scoreOf[s]; score != nil {
			out = append(out, domain.NormalizedRow{QID: q.ID, SubID: r.ID, Value: *score})
		} else {
			out = append(out, domain.NormalizedRow{QID: q.ID, SubID: r.ID, Value: s})
		}
	}
	return out
}

// ---------- matrix-fill(矩阵填空) ----------
// answer 形状:map[string]any(子行 id → string),无列。maxLength 用 UTF-16 计数(对齐前端)。

type matrixFillProps struct {
	Rows      []row `json:"rows"`
	MaxLength *int  `json:"maxLength"`
}

type matrixFill struct{}

func (matrixFill) Type() string { return "matrix-fill" }

func (matrixFill) Validate(q domain.Question, answer any) string {
	var p matrixFillProps
	unmarshalProps(q.Props, &p)
	ans := readMatrixAnswer(answer)

	rowIDs := map[string]bool{}
	for _, r := range p.Rows {
		rowIDs[r.ID] = true
	}

	for subID, val := range ans {
		// 与前端 handler.ts 同序:跳过空(nil/空串)→ 子行归属 → 类型 → 字数。
		if val == nil {
			continue
		}
		if s, ok := val.(string); ok && s == "" {
			continue
		}
		if !rowIDs[subID] {
			return "存在不属于本题的子项"
		}
		s, ok := val.(string)
		if !ok {
			return "答案格式应为文本"
		}
		if p.MaxLength != nil && utf16Len(s) > *p.MaxLength {
			return fmt.Sprintf("每行不超过 %d 个字符", *p.MaxLength)
		}
	}
	if q.Required {
		for _, r := range p.Rows {
			if !strAnswered(ans[r.ID]) {
				return "每个子项都需作答"
			}
		}
	}
	return ""
}

func (matrixFill) Normalize(q domain.Question, answer any) []domain.NormalizedRow {
	var p matrixFillProps
	unmarshalProps(q.Props, &p)
	ans := readMatrixAnswer(answer)

	var out []domain.NormalizedRow
	for _, r := range p.Rows {
		s, ok := ans[r.ID].(string)
		if !ok || s == "" {
			continue
		}
		out = append(out, domain.NormalizedRow{QID: q.ID, SubID: r.ID, Value: s})
	}
	return out
}

// ---------- matrix-slider(矩阵滑动条) ----------
// answer 形状:map[string]any(子行 id → float64,JSON 数字)。无列。未拖动的行无键。
// 不强校验步长整除(前端 range 已按 step 吸附,避免浮点误差误伤)。

type matrixSliderProps struct {
	Rows []row    `json:"rows"`
	Min  *float64 `json:"min"`
	Max  *float64 `json:"max"`
	Step *float64 `json:"step"`
}

type matrixSlider struct{}

func (matrixSlider) Type() string { return "matrix-slider" }

func (matrixSlider) Validate(q domain.Question, answer any) string {
	var p matrixSliderProps
	unmarshalProps(q.Props, &p)
	ans := readMatrixAnswer(answer)

	min := 0.0
	if p.Min != nil {
		min = *p.Min
	}
	max := 100.0
	if p.Max != nil {
		max = *p.Max
	}
	rowIDs := map[string]bool{}
	for _, r := range p.Rows {
		rowIDs[r.ID] = true
	}

	for subID, val := range ans {
		if val == nil {
			continue
		}
		if !rowIDs[subID] {
			return "存在不属于本题的子项"
		}
		n, ok := val.(float64)
		if !ok {
			return "答案应为数值"
		}
		if n < min || n > max {
			return fmt.Sprintf("数值应在 %s 到 %s 之间", numStr(min), numStr(max))
		}
	}
	if q.Required {
		for _, r := range p.Rows {
			if _, ok := ans[r.ID].(float64); !ok {
				return "每个子项都需作答"
			}
		}
	}
	return ""
}

func (matrixSlider) Normalize(q domain.Question, answer any) []domain.NormalizedRow {
	var p matrixSliderProps
	unmarshalProps(q.Props, &p)
	ans := readMatrixAnswer(answer)

	var out []domain.NormalizedRow
	for _, r := range p.Rows {
		n, ok := ans[r.ID].(float64)
		if !ok {
			continue
		}
		out = append(out, domain.NormalizedRow{QID: q.ID, SubID: r.ID, Value: n})
	}
	return out
}
