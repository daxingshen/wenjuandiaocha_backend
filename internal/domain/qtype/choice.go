// single-choice / multi-choice 的 engine 侧行为。复刻 packages/question-types/src/{single-choice,multi-choice}/handler.ts。
package qtype

import (
	"fmt"
	"strings"

	"wenjuandiaocha_backend/internal/domain"
)

// ---------- single-choice ----------
// answer 形状:string(选项 value),或带填空的对象形 {value, text}(选中允许填空的选项时)。
// 复刻前端 packages/question-types/src/single-choice/handler.ts。

// singleOption 单选选项:比 matrix 共用的 option 多一个 fill(允许填空)。
// style/image 是纯前端渲染配置,后端不关心但要能容忍反序列化(用 json.RawMessage 忽略)。
type singleOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Fill  *struct {
		Enabled  bool `json:"enabled"`
		Required bool `json:"required"`
	} `json:"fill"`
}

type singleChoiceProps struct {
	Options []singleOption `json:"options"`
}

type singleChoice struct{}

func (singleChoice) Type() string { return "single-choice" }

// readSingleAnswer 从答案取选项 value 与填空 text,兼容裸 string 与对象形 {value,text}。
// ok=false 表示格式非法(既非 string 也非带 string value 的对象)。
func readSingleAnswer(answer any) (value string, text string, ok bool) {
	switch a := answer.(type) {
	case string:
		return a, "", true
	case map[string]any:
		v, isStr := a["value"].(string)
		if !isStr {
			return "", "", false
		}
		t, _ := a["text"].(string)
		return v, t, true
	default:
		return "", "", false
	}
}

func (singleChoice) Validate(q domain.Question, answer any) string {
	var p singleChoiceProps
	unmarshalProps(q.Props, &p)
	value, text, ok := readSingleAnswer(answer)
	if !ok {
		return "答案格式应为单个选项"
	}
	var opt *singleOption
	for i := range p.Options {
		if p.Options[i].Value == value {
			opt = &p.Options[i]
			break
		}
	}
	if opt == nil {
		return "所选选项不存在"
	}
	// 带填空选项:必填时文本不能为空。
	if opt.Fill != nil && opt.Fill.Enabled && opt.Fill.Required && strings.TrimSpace(text) == "" {
		return "请填写补充内容"
	}
	return ""
}

func (singleChoice) Normalize(q domain.Question, answer any) []domain.NormalizedRow {
	value, text, ok := readSingleAnswer(answer)
	if !ok || value == "" {
		return nil
	}
	rows := []domain.NormalizedRow{{QID: q.ID, Value: value}}
	// 填空文本非空 → 追加一行(subId 'fill'),供统计/导出取原文。
	if text != "" {
		rows = append(rows, domain.NormalizedRow{QID: q.ID, SubID: "fill", Value: text})
	}
	return rows
}

// ---------- multi-choice ----------
// answer 形状:混合数组 []any,元素为裸 string(选项 value)或对象形 {value,text}
// (选中允许填空的选项)。复刻前端 packages/question-types/src/multi-choice/handler.ts。
// 关键:required(空数组)判断在此 handler 内,不在通用 ValidateSurvey(02-facts §6 陷阱)。

// multiOption 多选选项:同单选,可带 fill(允许填空)。style/image 是纯前端渲染配置,
// 后端不关心,反序列化时被忽略(结构体无对应字段即丢弃)。不再复用共享的裸 option。
type multiOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Fill  *struct {
		Enabled  bool `json:"enabled"`
		Required bool `json:"required"`
	} `json:"fill"`
}

type multiChoiceProps struct {
	Options []multiOption `json:"options"`
	Min     *int          `json:"min"`
	Max     *int          `json:"max"`
}

type multiChoice struct{}

func (multiChoice) Type() string { return "multi-choice" }

// readMultiElem 从混合数组的单个元素取选项 value 与填空 text,兼容裸 string 与对象形 {value,text}。
// ok=false 表示元素格式非法(既非 string 也非带 string value 的对象)。
func readMultiElem(el any) (value string, text string, ok bool) {
	switch e := el.(type) {
	case string:
		return e, "", true
	case map[string]any:
		v, isStr := e["value"].(string)
		if !isStr {
			return "", "", false
		}
		t, _ := e["text"].(string)
		return v, t, true
	default:
		return "", "", false
	}
}

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
	optOf := map[string]*multiOption{}
	for i := range p.Options {
		optOf[p.Options[i].Value] = &p.Options[i]
	}
	seen := map[string]bool{}
	for _, el := range arr {
		value, text, elOk := readMultiElem(el)
		opt := optOf[value]
		if !elOk || opt == nil {
			return "包含不存在的选项"
		}
		seen[value] = true
		// 带填空选项:必填时文本不能为空。
		if opt.Fill != nil && opt.Fill.Enabled && opt.Fill.Required && strings.TrimSpace(text) == "" {
			return "请填写补充内容"
		}
	}
	if len(seen) != len(arr) {
		return "选项不可重复"
	}
	// min/max 按选中项个数(数组长度)计。
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
	// 填空文本非空 → 追加一行,subId="<optValue>.fill"(与单选固定 "fill" 不同,
	// 多选可有多个填空项,须按选项 value 区分)。
	var out []domain.NormalizedRow
	for _, el := range arr {
		value, text, elOk := readMultiElem(el)
		if !elOk || value == "" {
			continue
		}
		out = append(out, domain.NormalizedRow{QID: q.ID, Value: value})
		if text != "" {
			out = append(out, domain.NormalizedRow{QID: q.ID, SubID: value + ".fill", Value: text})
		}
	}
	return out
}
