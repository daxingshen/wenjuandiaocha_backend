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

// TestSingleChoiceFill 覆盖「允许填空」:answer 为对象形 {value,text},前后端一致(决策1-A)。
func TestSingleChoiceFill(t *testing.T) {
	q := mkQ("q1", "single-choice", true, map[string]any{
		"options": []map[string]any{
			{"value": "a", "label": "A"},
			{"value": "other", "label": "其他", "fill": map[string]any{"enabled": true, "required": true}},
		},
	})
	h := handler(t, "single-choice")

	// 对象形合法答案通过(经 JSON 编解码后 answer 是 map[string]any)。
	okAns := map[string]any{"value": "other", "text": "具体用途"}
	if msg := h.Validate(q, okAns); msg != "" {
		t.Errorf("对象形合法答案应通过,得 %q", msg)
	}
	// fill.required 且文本为空 → 报填空必填。
	if msg := h.Validate(q, map[string]any{"value": "other", "text": ""}); msg != "请填写补充内容" {
		t.Errorf("必填填空空文本应报错,得 %q", msg)
	}
	// 裸 string 选中 other(无文本)也应报填空必填。
	if msg := h.Validate(q, "other"); msg != "请填写补充内容" {
		t.Errorf("裸 string 选中必填填空项应报错,得 %q", msg)
	}
	// 对象形不存在的选项 → 报错。
	if msg := h.Validate(q, map[string]any{"value": "z", "text": "x"}); msg != "所选选项不存在" {
		t.Errorf("对象形非法选项应报错,得 %q", msg)
	}
	// normalize:对象形带非空文本产两行(value + fill 子行)。
	rows := h.Normalize(q, okAns)
	if len(rows) != 2 || rows[0].Value != "other" || rows[1].SubID != "fill" || rows[1].Value != "具体用途" {
		t.Errorf("填空 normalize 应产两行(value+fill),得 %+v", rows)
	}
	// 对象形文本为空只产 value 一行。
	rows = h.Normalize(q, map[string]any{"value": "other", "text": ""})
	if len(rows) != 1 || rows[0].Value != "other" {
		t.Errorf("空文本应只产 value 一行,得 %+v", rows)
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

// TestMultiChoice_Fill 覆盖「多选带填空」:answer 是混合数组,元素为裸 string(选项 value)
// 或对象形 {value,text}(选中允许填空的选项)。前后端一致(决策1-A 的多选延伸)。
func TestMultiChoice_Fill(t *testing.T) {
	q := mkQ("q1", "multi-choice", true, map[string]any{
		"options": []map[string]any{
			{"value": "a", "label": "A"},
			{"value": "b", "label": "B"},
			{"value": "other", "label": "其他", "fill": map[string]any{"enabled": true, "required": true}},
		},
	})
	h := handler(t, "multi-choice")

	// 混合数组合法:裸 string + 带非空文本的对象形。
	okAns := []any{"a", map[string]any{"value": "other", "text": "具体渠道"}}
	if msg := h.Validate(q, okAns); msg != "" {
		t.Errorf("混合数组合法答案应通过,得 %q", msg)
	}
	// 旧的纯字符串数组仍然可用(other 无填空要求时才行,这里 other 必填,故只用 a/b)。
	if msg := h.Validate(q, []any{"a", "b"}); msg != "" {
		t.Errorf("纯字符串数组应通过,得 %q", msg)
	}
	// fill.required 且文本为空 → 报填空必填。
	if msg := h.Validate(q, []any{"a", map[string]any{"value": "other", "text": ""}}); msg != "请填写补充内容" {
		t.Errorf("必填填空空文本应报错,得 %q", msg)
	}
	// 裸 string 选中必填填空项(无文本)也应报填空必填。
	if msg := h.Validate(q, []any{"other"}); msg != "请填写补充内容" {
		t.Errorf("裸 string 选中必填填空项应报错,得 %q", msg)
	}
	// 含不存在的选项(对象形亦按 value 判断)。
	if msg := h.Validate(q, []any{map[string]any{"value": "z", "text": "x"}}); msg != "包含不存在的选项" {
		t.Errorf("对象形非法选项应报错,得 %q", msg)
	}
	// 重复(裸 string 与对象形 value 相同也算重复)。
	if msg := h.Validate(q, []any{"a", map[string]any{"value": "a", "text": ""}}); msg != "选项不可重复" {
		t.Errorf("重复选项应报错,得 %q", msg)
	}

	// normalize:每选中项一行;带非空文本的填空项追加 subId="<value>.fill" 行。
	rows := h.Normalize(q, okAns)
	if len(rows) != 3 {
		t.Fatalf("混合数组 normalize 应产 3 行(a + other + other.fill),得 %+v", rows)
	}
	if rows[0].Value != "a" || rows[1].Value != "other" {
		t.Errorf("前两行应为选中项 value,得 %+v", rows)
	}
	if rows[2].SubID != "other.fill" || rows[2].Value != "具体渠道" {
		t.Errorf("填空行 subId 应为 other.fill、value 为文本,得 %+v", rows[2])
	}
	// 空文本的填空项不产 fill 行。
	rows = h.Normalize(q, []any{map[string]any{"value": "other", "text": ""}})
	if len(rows) != 1 || rows[0].Value != "other" || rows[0].SubID != "" {
		t.Errorf("空文本填空项应只产 value 一行,得 %+v", rows)
	}
}

func TestMultiChoice_MinMaxMixed(t *testing.T) {
	// min/max 按选中项个数(数组长度)计,含对象形元素。
	q := mkQ("q1", "multi-choice", false, map[string]any{
		"options": []map[string]any{
			{"value": "a", "label": "A"},
			{"value": "b", "label": "B"},
			{"value": "other", "label": "其他", "fill": map[string]any{"enabled": true}},
		},
		"min": 2,
		"max": 2,
	})
	h := handler(t, "multi-choice")
	if msg := h.Validate(q, []any{map[string]any{"value": "other", "text": "x"}}); msg != "至少选择 2 项" {
		t.Errorf("少于 min 应报错,得 %q", msg)
	}
	if msg := h.Validate(q, []any{"a", "b", map[string]any{"value": "other", "text": "x"}}); msg != "最多选择 2 项" {
		t.Errorf("多于 max 应报错,得 %q", msg)
	}
	if msg := h.Validate(q, []any{"a", map[string]any{"value": "other", "text": "x"}}); msg != "" {
		t.Errorf("恰好 2 项应通过,得 %q", msg)
	}
}

func TestDropdown(t *testing.T) {
	q := mkQ("q1", "dropdown", true, map[string]any{
		"options":      []map[string]string{{"value": "a", "label": "A"}, {"value": "b", "label": "B"}},
		"defaultValue": "a",
	})
	h := handler(t, "dropdown")

	if msg := h.Validate(q, "a"); msg != "" {
		t.Errorf("合法选项应通过,得 %q", msg)
	}
	if msg := h.Validate(q, "z"); msg != "所选选项不存在" {
		t.Errorf("非法选项应报错,得 %q", msg)
	}
	if msg := h.Validate(q, 123.0); msg != "答案格式应为单个选项" {
		t.Errorf("非字符串应报格式错,得 %q", msg)
	}
	// 空串:交给通用必答层,dropdown 自身返回空(与单选一致语义)。
	if msg := h.Validate(q, ""); msg != "" {
		t.Errorf("空串应交给通用层,dropdown 自身应通过,得 %q", msg)
	}
	rows := h.Normalize(q, "a")
	if len(rows) != 1 || rows[0].Value != "a" {
		t.Errorf("normalize 应产一行 value=a,得 %+v", rows)
	}
	if rows := h.Normalize(q, ""); rows != nil {
		t.Errorf("空答案不产行,得 %+v", rows)
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

	minlen := mkQ("q4", "text-input", false, map[string]any{"minLength": 3})
	if msg := h.Validate(minlen, "ab"); msg != "至少 3 个字符" {
		t.Errorf("过短应报错,得 %q", msg)
	}

	// 缺省字段(旧问卷:无 format/min/max)照常通过。
	if msg := h.Validate(mkQ("q5", "text-input", false, map[string]any{}), "任意文本"); msg != "" {
		t.Errorf("旧问卷无字段应通过,得 %q", msg)
	}
}

// TestTextInputFormats 覆盖扩展的 11 项属性验证,与前端 shared/text-format.test.ts 同判。
func TestTextInputFormats(t *testing.T) {
	h := handler(t, "text-input")
	cases := []struct {
		format, value string
		ok            bool
	}{
		{"integer", "-42", true}, {"integer", "4.2", false},
		{"decimal", "-3.14", true}, {"decimal", "3.", false},
		{"date", "2026-02-28", true}, {"date", "2026-02-30", false}, {"date", "2026-2-8", false},
		{"age", "0", true}, {"age", "150", true}, {"age", "151", false}, {"age", "-1", false},
		{"province", "广东省", true}, {"province", "火星省", false},
		{"idcard", "110101199003076173", true}, {"idcard", "110101199003076170", false},
		{"zipcode", "100000", true}, {"zipcode", "1000", false},
		{"url", "https://example.com/x", true}, {"url", "example.com", false},
	}
	for _, c := range cases {
		q := mkQ("q", "text-input", false, map[string]any{"format": c.format})
		msg := h.Validate(q, c.value)
		if c.ok && msg != "" {
			t.Errorf("format=%s value=%q 应通过,得 %q", c.format, c.value, msg)
		}
		if !c.ok && msg == "" {
			t.Errorf("format=%s value=%q 应被拒", c.format, c.value)
		}
	}
}

// TestProvinceList 省份名单哨兵:34 个,与前端 provinces.ts 数量一致。
func TestProvinceList(t *testing.T) {
	if len(provinceList) != 34 {
		t.Errorf("省份名单应为 34 个,得 %d", len(provinceList))
	}
	if len(provinceSet) != 34 {
		t.Errorf("省份集合应为 34 个,得 %d", len(provinceSet))
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
	minq := mkQ("q2", "textarea", false, map[string]any{"minLength": 5})
	if msg := h.Validate(minq, "abc"); msg != "至少 5 个字符" {
		t.Errorf("过短应报错,得 %q", msg)
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

func TestMatrixMulti(t *testing.T) {
	props := map[string]any{
		"rows":    []map[string]string{{"id": "r1", "label": "行1"}, {"id": "r2", "label": "行2"}},
		"options": []map[string]string{{"value": "a", "label": "A"}, {"value": "b", "label": "B"}, {"value": "c", "label": "C"}},
		"min":     1,
		"max":     2,
	}
	q := mkQ("q1", "matrix-multi", false, props)
	h := handler(t, "matrix-multi")

	// 合法(非必答,空行跳过)
	if msg := h.Validate(q, map[string]any{"r1": []any{"a", "b"}}); msg != "" {
		t.Errorf("合法多选应通过,得 %q", msg)
	}
	// 超每行 max
	if msg := h.Validate(q, map[string]any{"r1": []any{"a", "b", "c"}}); msg != "每行最多选择 2 项" {
		t.Errorf("超每行 max 应报错,得 %q", msg)
	}
	// 重复
	if msg := h.Validate(q, map[string]any{"r1": []any{"a", "a"}}); msg != "选项不可重复" {
		t.Errorf("重复应报错,得 %q", msg)
	}
	// 非法列
	if msg := h.Validate(q, map[string]any{"r1": []any{"z"}}); msg != "包含不存在的选项" {
		t.Errorf("非法列应报错,得 %q", msg)
	}
	// 非数组行值
	if msg := h.Validate(q, map[string]any{"r1": "a"}); msg != "答案格式应为选项数组" {
		t.Errorf("非数组行值应报错,得 %q", msg)
	}
	// 必答:缺行
	qReq := mkQ("q1", "matrix-multi", true, props)
	if msg := h.Validate(qReq, map[string]any{"r1": []any{"a"}}); msg != "每个子项都需作答" {
		t.Errorf("必答缺行应报错,得 %q", msg)
	}
	// normalize:每行每项一行
	rows := h.Normalize(q, map[string]any{"r1": []any{"a", "b"}, "r2": []any{"c"}})
	if len(rows) != 3 || rows[0].SubID != "r1" || rows[2].SubID != "r2" || rows[2].Value != "c" {
		t.Errorf("矩阵多选 normalize 不符,得 %+v", rows)
	}
}

func TestMatrixScale(t *testing.T) {
	q := mkQ("q1", "matrix-scale", true, map[string]any{
		"rows": []map[string]any{{"id": "r1", "label": "行1"}, {"id": "r2", "label": "行2"}},
		"options": []map[string]any{
			{"value": "s1", "label": "差", "score": 1},
			{"value": "s5", "label": "好", "score": 5},
		},
		"level": 5,
	})
	h := handler(t, "matrix-scale")

	if msg := h.Validate(q, map[string]any{"r1": "s1", "r2": "s5"}); msg != "" {
		t.Errorf("合法量表应通过,得 %q", msg)
	}
	if msg := h.Validate(q, map[string]any{"r1": "s9", "r2": "s5"}); msg != "所选选项不存在" {
		t.Errorf("非法量级应报错,得 %q", msg)
	}
	if msg := h.Validate(q, map[string]any{"r1": "s1"}); msg != "每个子项都需作答" {
		t.Errorf("必答缺行应报错,得 %q", msg)
	}
	// normalize 产出分值(float64),不是列 value 字符串。
	rows := h.Normalize(q, map[string]any{"r1": "s1", "r2": "s5"})
	if len(rows) != 2 || rows[0].Value != 1.0 || rows[1].Value != 5.0 {
		t.Errorf("量表 normalize 应产分值 1,5,得 %+v", rows)
	}
}

func TestMatrixFill(t *testing.T) {
	props := map[string]any{
		"rows":      []map[string]string{{"id": "r1", "label": "手机"}, {"id": "r2", "label": "邮箱"}},
		"maxLength": 5,
	}
	q := mkQ("q1", "matrix-fill", false, props)
	h := handler(t, "matrix-fill")

	if msg := h.Validate(q, map[string]any{"r1": "abc"}); msg != "" {
		t.Errorf("合法填空应通过,得 %q", msg)
	}
	if msg := h.Validate(q, map[string]any{"r1": "abcdef"}); msg != "每行不超过 5 个字符" {
		t.Errorf("超长应报错,得 %q", msg)
	}
	if msg := h.Validate(q, map[string]any{"rX": "a"}); msg != "存在不属于本题的子项" {
		t.Errorf("非法子行应报错,得 %q", msg)
	}
	qReq := mkQ("q1", "matrix-fill", true, props)
	if msg := h.Validate(qReq, map[string]any{"r1": "abc"}); msg != "每个子项都需作答" {
		t.Errorf("必答缺行应报错,得 %q", msg)
	}
	rows := h.Normalize(q, map[string]any{"r1": "abc", "r2": ""})
	if len(rows) != 1 || rows[0].SubID != "r1" || rows[0].Value != "abc" {
		t.Errorf("填空 normalize 不符,得 %+v", rows)
	}
}

// TestMultiFill 与前端 multi-fill/__tests__/handler.test.ts 同判:每框独立 format/长度、
// 必答每框非空(含空对象不绕过)、越界框、normalize 每已答框一行。
func TestMultiFill(t *testing.T) {
	props := map[string]any{
		"blanks": []map[string]any{
			{"id": "b1", "label": "收货人"},
			{"id": "b2", "label": "手机号", "format": "phone"},
			{"id": "b3", "label": "省份", "format": "province"},
		},
	}
	q := mkQ("q1", "multi-fill", false, props)
	qReq := mkQ("q1", "multi-fill", true, props)
	h := handler(t, "multi-fill")

	if msg := h.Validate(q, map[string]any{"b2": "13800138000"}); msg != "" {
		t.Errorf("合法手机应通过,得 %q", msg)
	}
	if msg := h.Validate(q, map[string]any{"b2": "123"}); msg != "手机号格式不正确" {
		t.Errorf("非法手机应报错,得 %q", msg)
	}
	if msg := h.Validate(q, map[string]any{"b3": "火星省"}); msg != "请选择省份" {
		t.Errorf("非法省份应报错,得 %q", msg)
	}
	if msg := h.Validate(q, map[string]any{"bx": "x"}); msg != "存在不属于本题的填空框" {
		t.Errorf("越界框应报错,得 %q", msg)
	}
	// 必答:每框非空;空对象不绕过。
	if msg := h.Validate(qReq, map[string]any{"b1": "张三", "b2": "13800138000", "b3": "广东省"}); msg != "" {
		t.Errorf("必答全填应通过,得 %q", msg)
	}
	if msg := h.Validate(qReq, map[string]any{"b1": "张三"}); msg != "每个填空框都需作答" {
		t.Errorf("必答缺框应报错,得 %q", msg)
	}
	if msg := h.Validate(qReq, map[string]any{}); msg != "每个填空框都需作答" {
		t.Errorf("必答空对象应被拦,得 %q", msg)
	}
	// 每框独立字数范围。
	lenProps := map[string]any{"blanks": []map[string]any{{"id": "b1", "minLength": 3, "maxLength": 5}}}
	ql := mkQ("q2", "multi-fill", false, lenProps)
	if msg := h.Validate(ql, map[string]any{"b1": "ab"}); msg != "至少 3 个字符" {
		t.Errorf("过短应报错,得 %q", msg)
	}
	if msg := h.Validate(ql, map[string]any{"b1": "abcdef"}); msg != "不超过 5 个字符" {
		t.Errorf("超长应报错,得 %q", msg)
	}
	// normalize:每已答框一行,按 blanks 顺序,空框不产行。
	rows := h.Normalize(q, map[string]any{"b3": "广东省", "b1": "张三", "b2": ""})
	if len(rows) != 2 || rows[0].SubID != "b1" || rows[0].Value != "张三" || rows[1].SubID != "b3" {
		t.Errorf("multi-fill normalize 不符,得 %+v", rows)
	}
}

func TestMatrixSlider(t *testing.T) {
	props := map[string]any{
		"rows": []map[string]string{{"id": "r1", "label": "价格"}, {"id": "r2", "label": "质量"}},
		"min":  0,
		"max":  100,
		"step": 5,
	}
	q := mkQ("q1", "matrix-slider", false, props)
	h := handler(t, "matrix-slider")

	// JSON 数字 → float64
	if msg := h.Validate(q, map[string]any{"r1": 60.0}); msg != "" {
		t.Errorf("合法滑动应通过,得 %q", msg)
	}
	if msg := h.Validate(q, map[string]any{"r1": 200.0}); msg != "数值应在 0 到 100 之间" {
		t.Errorf("越界应报错,得 %q", msg)
	}
	if msg := h.Validate(q, map[string]any{"r1": "x"}); msg != "答案应为数值" {
		t.Errorf("非数值应报错,得 %q", msg)
	}
	if msg := h.Validate(q, map[string]any{"rX": 10.0}); msg != "存在不属于本题的子项" {
		t.Errorf("非法子行应报错,得 %q", msg)
	}
	// 必答:未拖动的行(无键)
	qReq := mkQ("q1", "matrix-slider", true, props)
	if msg := h.Validate(qReq, map[string]any{"r1": 60.0}); msg != "每个子项都需作答" {
		t.Errorf("必答缺行应报错,得 %q", msg)
	}
	rows := h.Normalize(q, map[string]any{"r1": 60.0, "r2": 80.0})
	if len(rows) != 2 || rows[0].Value != 60.0 || rows[1].Value != 80.0 {
		t.Errorf("滑动 normalize 不符,得 %+v", rows)
	}
}
