// 题型注册表。对应 PRD §9 约束 2(题型 type+props,加题型零 DDL)。
// 注册表住在 domain(类比前端 engine 持有 registry);具体题型 handler 在 qtype 子包,
// 由 qtype.RegisterAll() 反向注册进来。依赖单向 qtype → domain,无环。
package domain

import "sync"

// Handler 题型处理器:非 UI 行为契约。对齐前端 QuestionTypeHandler 的非 UI 部分。
type Handler interface {
	// Type 题型唯一标识,与 Question.Type 对应。
	Type() string
	// Validate 校验一道题的答案,返回错误信息;空串表示通过。
	// 注意:multi-choice / matrix 的必答判断在各自 handler 内(空数组/空对象在通用层算已答)。
	Validate(q Question, answer any) string
	// Normalize 规范化答案为若干行(多选→多行,未答→空)。
	Normalize(q Question, answer any) []NormalizedRow
}

var (
	regMu    sync.RWMutex
	handlers = map[string]Handler{}
)

// RegisterHandler 注册一个题型处理器。重复注册同 type 覆盖。
func RegisterHandler(h Handler) {
	regMu.Lock()
	defer regMu.Unlock()
	handlers[h.Type()] = h
}

// GetHandler 取处理器;未注册返回 nil, false。
func GetHandler(t string) (Handler, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	h, ok := handlers[t]
	return h, ok
}
