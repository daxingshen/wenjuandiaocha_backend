// Package domain 是星卷后端的纯函数核心:问卷类型 + 逻辑求值 + 校验 + 规范化 + 题型 handler。
// 零框架依赖(不 import gin/pgx),是前端 packages/engine 的后端孪生。
//
// 本文件是数据模型,严格对齐前端 packages/engine/src/schema.ts 的字段名与形状,
// 前端无适配层——wire JSON 直接 Unmarshal 成这些类型。
package domain

import "encoding/json"

// SurveyType 问卷形态。类型是一个字段,不是六套系统(PRD §9 约束 1)。
type SurveyType string

const (
	SurveySurvey    SurveyType = "survey"    // 问卷调查(MVP 唯一实现的形态)
	SurveyExam      SurveyType = "exam"      // 在线考试
	SurveyVote      SurveyType = "vote"      // 投票评选
	SurveySignup    SurveyType = "signup"    // 报名表单
	SurveyAssess    SurveyType = "assess"    // 心理测评
	SurveyReview360 SurveyType = "review360" // 360 评估
)

// Question 一道题。核心不认识具体题型,题型细节全在 Props(约束 2)。
type Question struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Title    string          `json:"title"`
	Required bool            `json:"required,omitempty"`
	// Props 题型专属配置,核心层不解释其结构;各题型 handler 按需解析。
	Props json.RawMessage `json:"props"`
}

// SurveySchema 一份问卷(某个版本)。
type SurveySchema struct {
	ID    string     `json:"id"`
	Type  SurveyType `json:"type"`
	Title string     `json:"title"`
	// Version schema 版本号。发布后改题递增,旧答卷按旧版本解释(约束 5)。
	Version   int         `json:"version"`
	Questions []Question  `json:"questions"`
	// Rules 逻辑规则,独立于题目(约束 3)。
	Rules []LogicRule `json:"rules"`
}

// ConditionOp 条件比较运算符。语义集中在 logic.go 的 evalCondition。
type ConditionOp string

const (
	OpEq       ConditionOp = "eq"       // 等于
	OpNe       ConditionOp = "ne"       // 不等于
	OpIncludes ConditionOp = "includes" // 数组答案包含 value(多选)
	OpGt       ConditionOp = "gt"       // 大于(数值)
	OpLt       ConditionOp = "lt"       // 小于(数值)
	OpAnswered ConditionOp = "answered" // 已作答(value 忽略)
	OpEmpty    ConditionOp = "empty"    // 未作答(value 忽略)
)

// Condition 一条逻辑规则的条件:某题(或其矩阵子行)的答案与某值的比较。
type Condition struct {
	QID string `json:"qid"`
	// SubID 矩阵子行 id:引用某题的某一子行;标量题省略。
	SubID string      `json:"subId,omitempty"`
	Op    ConditionOp `json:"op"`
	// Value 比较值,语义由 Op 决定;用 any 承载 JSON 任意标量/数组。
	Value any `json:"value"`
}

// Combinator 条件组合子。
type Combinator string

const (
	CombAnd Combinator = "AND"
	CombOr  Combinator = "OR"
)

// RuleActionType 规则动作类型。新增逻辑类型 = 新增一个 type,不改结构(约束 3)。
// MVP 只实现 show / hide;jump / end / pipe 预留。
type RuleActionType string

const (
	ActionShow RuleActionType = "show"
	ActionHide RuleActionType = "hide"
	ActionJump RuleActionType = "jump"
	ActionEnd  RuleActionType = "end"
	ActionPipe RuleActionType = "pipe"
)

// RuleAction 规则动作。
type RuleAction struct {
	Type RuleActionType `json:"type"`
	// Target 动作目标题目 id(show/hide/jump 用)。
	Target string `json:"target"`
}

// LogicRule 一条逻辑规则:[条件组] --combinator--> [动作]。
type LogicRule struct {
	ID         string      `json:"id"`
	Conditions []Condition `json:"conditions"`
	Combinator Combinator  `json:"combinator"`
	Action     RuleAction  `json:"action"`
}

// Answers 作答答案:题目 id → 答案值。值的结构由题型决定。
type Answers map[string]any

// NormalizedRow 一道题规范化后的一行(喂统计与交叉分析,约束 4)。
// 对齐前端 registry.ts 的 NormalizedRow:{ qid, subId?, value: string|number }。
type NormalizedRow struct {
	QID string `json:"qid"`
	// SubID 矩阵子行 id(每子行一行);标量题为空。
	SubID string `json:"subId,omitempty"`
	// Value 规范化标量值:选项 value / 数值 / 文本。string 或 float64。
	Value any `json:"value"`
}
