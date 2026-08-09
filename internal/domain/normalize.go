// 答案规范化。对应 PRD §9 约束 4(答卷双写),复刻前端 packages/engine/src/normalize.ts。
//
// 把一份答卷摊平成规范化行,喂统计与交叉分析。多选题一个选项一行。
// 隐藏题不产出行。委托各题型 handler 的 Normalize。
package domain

// NormalizeSurvey 把整份答卷规范化为行集合。
func NormalizeSurvey(schema SurveySchema, answers Answers) []NormalizedRow {
	hidden := Evaluate(schema.Rules, answers).Hidden
	var rows []NormalizedRow
	for _, q := range schema.Questions {
		if hidden[q.ID] {
			continue
		}
		answer, ok := answers[q.ID]
		if !ok || !isAnswered(answer) {
			continue
		}
		h, hok := GetHandler(q.Type)
		if !hok {
			continue
		}
		rows = append(rows, h.Normalize(q, answer)...)
	}
	return rows
}
