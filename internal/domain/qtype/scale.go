// scale(量表题)的 engine 侧行为。复刻 packages/question-types/src/scale/handler.ts。
// answer 形状:number(整数刻度)。JSON 数字进 Go 是 float64,需判整数值(复刻 Number.isInteger)。
package qtype

import (
	"fmt"
	"math"

	"wenjuandiaocha_backend/internal/domain"
)

type scaleProps struct {
	Min *float64 `json:"min"`
	Max *float64 `json:"max"`
}

// readScaleRange 复刻 readProps 兜底:min 默认 1,max 默认 5。
func (scaleProps) readRange(q domain.Question) (min, max float64) {
	var p scaleProps
	unmarshalProps(q.Props, &p)
	min, max = 1, 5
	if p.Min != nil {
		min = *p.Min
	}
	if p.Max != nil {
		max = *p.Max
	}
	return
}

type scale struct{}

func (scale) Type() string { return "scale" }

func (scale) Validate(q domain.Question, answer any) string {
	min, max := scaleProps{}.readRange(q)
	n, ok := answer.(float64)
	if !ok || n != math.Trunc(n) { // 非数字 或 非整数
		return "答案应为整数刻度值"
	}
	if n < min || n > max {
		return fmt.Sprintf("刻度值应在 %s 到 %s 之间", numStr(min), numStr(max))
	}
	return ""
}

func (scale) Normalize(q domain.Question, answer any) []domain.NormalizedRow {
	n, ok := answer.(float64)
	if !ok {
		return nil
	}
	return []domain.NormalizedRow{{QID: q.ID, Value: n}}
}

// numStr 把 min/max 打成与前端一致的整数字面(5 而非 5.0);非整数保留小数。
func numStr(f float64) string {
	if f == math.Trunc(f) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%g", f)
}
