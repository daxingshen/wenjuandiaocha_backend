// Package qtype 是 6 个 MVP 题型的 engine 侧行为(非 UI):validate/normalize。
// 复刻前端 packages/question-types/src/*/handler.ts,逐条对齐(见 wiki 02-facts §6)。
// 依赖单向 qtype → domain:各 handler 实现 domain.Handler,由 RegisterAll 注册进 domain 注册表。
package qtype

import "wenjuandiaocha_backend/internal/domain"

// RegisterAll 注册全部 6 个 MVP 题型。main / 测试启动时调一次。
func RegisterAll() {
	domain.RegisterHandler(singleChoice{})
	domain.RegisterHandler(multiChoice{})
	domain.RegisterHandler(scale{})
	domain.RegisterHandler(textInput{})
	domain.RegisterHandler(textarea{})
	domain.RegisterHandler(matrixSingle{})
}
