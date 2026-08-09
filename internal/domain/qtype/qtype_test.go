// 6 题型对拍测试。用例对齐前端 handler.test.ts 的行为(尤其 02-facts §6 的双层必答陷阱)。
// 答案值用「经 JSON 编解码后的形状」构造:数字 float64、数组 []any、矩阵 map[string]any。
package qtype

import (
	"encoding/json"
	"testing"

	"wenjuandiaocha_backend/internal/domain"
)

func init() { RegisterAll() }

// mkQ 造一道题,props 用字面 map 转 json.RawMessage。
func mkQ(id, typ string, required bool, props map[string]any) domain.Question {
	raw, _ := json.Marshal(props)
	return domain.Question{ID: id, Type: typ, Required: required, Props: raw}
}

func handler(t *testing.T, typ string) domain.Handler {
	h, ok := domain.GetHandler(typ)
	if !ok {
		t.Fatalf("题型 %s 未注册", typ)
	}
	return h
}

func TestSingleChoice(t *testing.T) {
	q := mkQ("q1", "single-choice", true, map[string]any{
		"options": []map[string]string{{"value": "a", "label": "A"}, {"value": "b", "label": "B"}},
	})
	h := handler(t, "single-choice")

	if msg := h.Validate(q, "a"); msg != "" {
		t.Errorf("合法选项应通过,得 %q", msg)
	}
	if msg := h.Validate(q, "z"); msg != "所选选项不存在" {
		t.Errorf("非法选项应报错,得 %q", msg)
	}
	if msg := h.Validate(q, 123.0); msg != "答案格式应为单个选项" {
		t.Errorf("非字符串应报格式错,得 %q", msg)
	}
	rows := h.Normalize(q, "a")
	if len(rows) != 1 || rows[0].Value != "a" {
		t.Errorf("normalize 应产一行 value=a,得 %+v", rows)
	}
	if rows := h.Normalize(q, ""); rows != nil {
		t.Errorf("空答案不产行,得 %+v", rows)
	}
}

func TestMultiChoice_RequiredEmptyArray(t *testing.T) {
	// 陷阱核心:空数组在通用层算「已答」,必答判断必须在 handler 内。
	q := mkQ("q1", "multi-choice", true, map[string]any{
		"options": []map[string]string{{"value": "a", "label": "A"}, {"value": "b", "label": "B"}},
	})
	h := handler(t, "multi-choice")

	if msg := h.Validate(q, []any{}); msg != "此题为必答" {
		t.Errorf("必答空数组应报必答,得 %q", msg)
	}
	if msg := h.Validate(q, []any{"a"}); msg != "" {
		t.Errorf("选一项应通过,得 %q", msg)
	}
	if msg := h.Validate(q, []any{"a", "z"}); msg != "包含不存在的选项" {
		t.Errorf("含非法选项应报错,得 %q", msg)
	}
	if msg := h.Validate(q, []any{"a", "a"}); msg != "选项不可重复" {
		t.Errorf("重复选项应报错,得 %q", msg)
	}
	if msg := h.Validate(q, "a"); msg != "答案格式应为选项数组" {
		t.Errorf("非数组应报格式错,得 %q", msg)
	}
	// normalize:一项一行
	rows := h.Normalize(q, []any{"a", "b"})
	if len(rows) != 2 {
		t.Errorf("多选 normalize 应拆 2 行,得 %+v", rows)
	}
}

func TestMultiChoice_MinMax(t *testing.T) {
	q := mkQ("q1", "multi-choice", false, map[string]any{
		"options": []map[string]string{{"value": "a", "label": "A"}, {"value": "b", "label": "B"}, {"value": "c", "label": "C"}},
		"min":     2,
		"max":     2,
	})
	h := handler(t, "multi-choice")
	if msg := h.Validate(q, []any{"a"}); msg != "至少选择 2 项" {
		t.Errorf("少于 min 应报错,得 %q", msg)
	}
	if msg := h.Validate(q, []any{"a", "b", "c"}); msg != "最多选择 2 项" {
		t.Errorf("多于 max 应报错,得 %q", msg)
	}
	if msg := h.Validate(q, []any{"a", "b"}); msg != "" {
		t.Errorf("恰好 2 项应通过,得 %q", msg)
	}
}

func TestScale(t *testing.T) {
	q := mkQ("q1", "scale", true, map[string]any{"min": 1, "max": 5})
	h := handler(t, "scale")

	if msg := h.Validate(q, 3.0); msg != "" {
		t.Errorf("范围内整数应通过,得 %q", msg)
	}
	if msg := h.Validate(q, 7.0); msg != "刻度值应在 1 到 5 之间" {
		t.Errorf("越界应报错,得 %q", msg)
	}
	if msg := h.Validate(q, 3.5); msg != "答案应为整数刻度值" {
		t.Errorf("非整数应报错,得 %q", msg)
	}
	if msg := h.Validate(q, "3"); msg != "答案应为整数刻度值" {
		t.Errorf("字符串应报错,得 %q", msg)
	}
	rows := h.Normalize(q, 4.0)
	if len(rows) != 1 || rows[0].Value != 4.0 {
		t.Errorf("scale normalize 应产 number 一行,得 %+v", rows)
	}
}

func TestScale_DefaultRange(t *testing.T) {
	// props 空 → min=1 max=5 兜底
	q := mkQ("q1", "scale", false, map[string]any{})
	h := handler(t, "scale")
	if msg := h.Validate(q, 5.0); msg != "" {
		t.Errorf("默认范围 5 应通过,得 %q", msg)
	}
	if msg := h.Validate(q, 6.0); msg != "刻度值应在 1 到 5 之间" {
		t.Errorf("默认范围外应报错,得 %q", msg)
	}
}

func TestTextInput(t *testing.T) {
	h := handler(t, "text-input")

	email := mkQ("q1", "text-input", false, map[string]any{"format": "email"})
	if msg := h.Validate(email, "a@b.com"); msg != "" {
		t.Errorf("合法邮箱应通过,得 %q", msg)
	}
	if msg := h.Validate(email, "nope"); msg != "邮箱格式不正确" {
		t.Errorf("非法邮箱应报错,得 %q", msg)
	}

	phone := mkQ("q2", "text-input", false, map[string]any{"format": "phone"})
	if msg := h.Validate(phone, "13800138000"); msg != "" {
		t.Errorf("合法手机应通过,得 %q", msg)
	}
	if msg := h.Validate(phone, "123"); msg != "手机号格式不正确" {
		t.Errorf("非法手机应报错,得 %q", msg)
	}

	maxlen := mkQ("q3", "text-input", false, map[string]any{"maxLength": 3})
	if msg := h.Validate(maxlen, "abcd"); msg != "不超过 3 个字符" {
		t.Errorf("超长应报错,得 %q", msg)
	}
	if msg := h.Validate(maxlen, "abc"); msg != "" {
		t.Errorf("恰好 3 应通过,得 %q", msg)
	}
}

func TestTextarea(t *testing.T) {
	h := handler(t, "textarea")
	q := mkQ("q1", "textarea", false, map[string]any{"maxLength": 5})
	if msg := h.Validate(q, "hello world"); msg != "不超过 5 个字符" {
		t.Errorf("超长应报错,得 %q", msg)
	}
	if msg := h.Validate(q, 42.0); msg != "答案格式应为文本" {
		t.Errorf("非文本应报错,得 %q", msg)
	}
	rows := h.Normalize(q, "hi")
	if len(rows) != 1 || rows[0].Value != "hi" {
		t.Errorf("textarea normalize 存原文,得 %+v", rows)
	}
}

func TestMatrixSingle(t *testing.T) {
	q := mkQ("q1", "matrix-single", true, map[string]any{
		"rows":    []map[string]string{{"id": "r1", "label": "行1"}, {"id": "r2", "label": "行2"}},
		"options": []map[string]string{{"value": "a", "label": "A"}, {"value": "b", "label": "B"}},
	})
	h := handler(t, "matrix-single")

	// 必答:缺子行 → 报错(handler 内判断)
	if msg := h.Validate(q, map[string]any{"r1": "a"}); msg != "每个子项都需作答" {
		t.Errorf("必答缺子行应报错,得 %q", msg)
	}
	// 全答 → 通过
	if msg := h.Validate(q, map[string]any{"r1": "a", "r2": "b"}); msg != "" {
		t.Errorf("全答应通过,得 %q", msg)
	}
	// 非法子行
	if msg := h.Validate(q, map[string]any{"r1": "a", "r2": "b", "rX": "a"}); msg != "存在不属于本题的子项" {
		t.Errorf("非法子行应报错,得 %q", msg)
	}
	// 非法列值
	if msg := h.Validate(q, map[string]any{"r1": "z", "r2": "b"}); msg != "所选选项不存在" {
		t.Errorf("非法列值应报错,得 %q", msg)
	}
	// normalize:按 rows 顺序,每已答子行一行带 subId
	rows := h.Normalize(q, map[string]any{"r2": "b", "r1": "a"})
	if len(rows) != 2 || rows[0].SubID != "r1" || rows[1].SubID != "r2" {
		t.Errorf("矩阵 normalize 应按 rows 顺序产 r1,r2,得 %+v", rows)
	}
	if rows[0].Value != "a" || rows[1].Value != "b" {
		t.Errorf("矩阵行 value 不符,得 %+v", rows)
	}
}
