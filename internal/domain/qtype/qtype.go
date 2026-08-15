// Package qtype 是各 MVP 题型的 engine 侧行为(非 UI):validate/normalize。
// 复刻前端 packages/question-types/src/*/handler.ts,逐条对齐(见 wiki 02-facts §6)。
// 依赖单向 qtype → domain:各 handler 实现 domain.Handler,由 RegisterAll 注册进 domain 注册表。
package qtype

import "wenjuandiaocha_backend/internal/domain"

// RegisterAll 注册全部题型(基础 5 + 矩阵 5)。main / 测试启动时调一次。
func RegisterAll() {
	domain.RegisterHandler(singleChoice{})
	domain.RegisterHandler(multiChoice{})
	domain.RegisterHandler(scale{})
	domain.RegisterHandler(textInput{})
	domain.RegisterHandler(textarea{})
	domain.RegisterHandler(matrixSingle{})
	domain.RegisterHandler(matrixMulti{})
	domain.RegisterHandler(matrixScale{})
	domain.RegisterHandler(matrixFill{})
	domain.RegisterHandler(matrixSlider{})
}
