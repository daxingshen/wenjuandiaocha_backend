// 校验器。对应 PRD §4.2,复刻前端 packages/engine/src/validate.ts。
//
// 组合「隐藏题跳过校验」(逻辑求值结果)与「每题型自己的 validate」(注册表)。
// 后端提交时必须重跑本流程,永不信任客户端(决策 6 安全底线)。
package domain

// ValidationError 单条校验错误。对齐前端 { qid, message }。
type ValidationError struct {
	QID     string `json:"qid"`
	Message string `json:"message"`
}

// ValidateSurvey 校验整份答卷。被逻辑隐藏的题跳过(隐藏题不该要求作答)。
// 返回全部错误;空切片表示通过。
func ValidateSurvey(schema SurveySchema, answers Answers) []ValidationError {
	hidden := Evaluate(schema.Rules, answers).Hidden
	var errs []ValidationError
	for _, q := range schema.Questions {
		if hidden[q.ID] {
			continue
		}
		answer := answers[q.ID]
		answered := isAnswered(answer)

		if q.Required && !answered {
			errs = append(errs, ValidationError{QID: q.ID, Message: "此题为必答"})
			continue
		}
		if !answered {
			continue
		}
		h, ok := GetHandler(q.Type)
		if !ok {
			continue
		}
		if msg := h.Validate(q, answer); msg != "" {
			errs = append(errs, ValidationError{QID: q.ID, Message: msg})
		}
	}
	return errs
}
