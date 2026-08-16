// 逻辑求值器。对应 PRD §9 约束 3,复刻前端 packages/engine/src/logic.ts 的行为。
//
// 核心是纯函数 Evaluate(rules, answers):输入声明式规则 + 当前答案,输出每题可见性。
// 与前端各实现一份,靠 golden-vectors.json 锁一致(frontend.md 决策 6)。
//
// 跨语言要点:JSON 数字进 Go 是 float64,answer 与 condition.value 都经 encoding/json,
// 所以 eq/ne 的相等比较用「同为 JSON 标量」的语义(见 jsEqual),不能裸用 Go ==(会因
// int/float64 类型不同而误判)。
package domain

// EvalResult 求值结果:被隐藏的题目 id 集合。默认可见,规则可置为隐藏。
type EvalResult struct {
	Hidden map[string]bool
}

// isAnswered 复刻 logic.ts 的 answered 语义:非 undefined/null/空串即已答。
// Go 侧:nil 或 空 string 视为未答;其余(含空数组、空对象、0、false)视为已答——
// 与 JS `a!==undefined && a!==null && a!==''` 一致(空数组/空对象在 JS 里也算已答)。
func isAnswered(a any) bool {
	if a == nil {
		return false
	}
	if s, ok := a.(string); ok && s == "" {
		return false
	}
	return true
}

// pickComparable 复刻 logic.ts 的取值:
//   - 有 subId:钻入 answers[qid][subId];该题未答或非「非数组对象」则子行取 nil。
//   - 无 subId:若答案是「带自有 value 字段的对象」(单选带填空 {value,text}),取其 value;
//     否则原样返回(裸 string/number、数组、矩阵整题对象等)。这样标量题带元数据后,
//     既有 eq/ne/answered 仍按选项 value 比较,不降级(见 wiki 03-design 决策1-A)。
func pickComparable(raw any, subID string) any {
	if subID != "" {
		m, ok := raw.(map[string]any)
		if !ok {
			return nil
		}
		return m[subID]
	}
	if m, ok := raw.(map[string]any); ok {
		if v, has := m["value"]; has {
			return v
		}
	}
	return raw
}

// pickElemValue 复刻 logic.ts 的同名助手:从多选答案的单个元素取比较值。
// 裸 string/number 原样;带自有 value 字段的对象(多选带填空项 {value,text})取其 value。
// 用于 includes 逐元素比较——否则对象元素永远等不上字符串 condition.value。
func pickElemValue(el any) any {
	if m, ok := el.(map[string]any); ok {
		if v, has := m["value"]; has {
			return v
		}
	}
	return el
}

// jsEqual 复刻 JS 严格相等 === 在「经过 JSON 编解码的标量」上的行为。
// JSON 数字统一是 float64;字符串比字符串;布尔比布尔;null 已在调用前排除。
func jsEqual(a, b any) bool {
	switch av := a.(type) {
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case float64:
		bv, ok := toFloat(b)
		return ok && av == bv
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	default:
		return false
	}
}

// toFloat 把 JSON 数值(float64;兼容 int/int64,防调用方直接塞 Go 数字)转 float64。
func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

// evalCondition 判断单个条件是否成立。空值/未作答语义集中在此,是前后端最易漂移处。
func evalCondition(c Condition, answers Answers) bool {
	raw := answers[c.QID]
	a := pickComparable(raw, c.SubID)
	answered := isAnswered(a)

	switch c.Op {
	case OpAnswered:
		return answered
	case OpEmpty:
		return !answered
	case OpEq:
		return jsEqual(a, c.Value)
	case OpNe:
		return !jsEqual(a, c.Value)
	case OpIncludes:
		arr, ok := a.([]any)
		if !ok {
			return false
		}
		// 元素可能是裸 value(string),也可能是带填空的对象 {value,text};
		// 逐元素钻取其比较值(对象取 .value)后再比,才能匹配到带填空的选中项。
		for _, item := range arr {
			if jsEqual(pickElemValue(item), c.Value) {
				return true
			}
		}
		return false
	case OpGt:
		av, aok := a.(float64)
		bv, bok := toFloat(c.Value)
		return aok && bok && av > bv
	case OpLt:
		av, aok := a.(float64)
		bv, bok := toFloat(c.Value)
		return aok && bok && av < bv
	default:
		return false
	}
}

// evalConditions 按组合子聚合一组条件。空条件组约定为不触发(false)。
func evalConditions(conditions []Condition, comb Combinator, answers Answers) bool {
	if len(conditions) == 0 {
		return false
	}
	if comb == CombAnd {
		for _, c := range conditions {
			if !evalCondition(c, answers) {
				return false
			}
		}
		return true
	}
	// OR(默认):一真即命中
	for _, c := range conditions {
		if evalCondition(c, answers) {
			return true
		}
	}
	return false
}

// Evaluate 求值全部规则,得出派生可见性。
// MVP 只处理 show/hide:hide 命中 → 隐藏 target;show 命中 → 显式可见(覆盖同题 hide)。
// jump/pipe/end 预留,当前忽略。
func Evaluate(rules []LogicRule, answers Answers) EvalResult {
	hidden := map[string]bool{}
	forcedShow := map[string]bool{}
	for _, r := range rules {
		if !evalConditions(r.Conditions, r.Combinator, answers) {
			continue
		}
		switch r.Action.Type {
		case ActionHide:
			hidden[r.Action.Target] = true
		case ActionShow:
			forcedShow[r.Action.Target] = true
		}
	}
	for id := range forcedShow {
		delete(hidden, id)
	}
	return EvalResult{Hidden: hidden}
}
