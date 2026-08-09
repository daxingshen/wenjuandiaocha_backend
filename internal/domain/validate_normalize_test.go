// ValidateSurvey / NormalizeSurvey 编排测试:隐藏题跳过、必答、委托 handler、双写规范化行。
// 用 qtype 真实 handler(通过 blank import 注册),验证整链。
package domain_test

import (
	"encoding/json"
	"testing"

	"wenjuandiaocha_backend/internal/domain"
	"wenjuandiaocha_backend/internal/domain/qtype"
)

func init() { qtype.RegisterAll() }

func rawProps(m map[string]any) json.RawMessage {
	b, _ := json.Marshal(m)
	return b
}

// 造一份含显隐逻辑的问卷:q1 单选,选 no 则隐藏 q2 量表。
func demoSchema() domain.SurveySchema {
	return domain.SurveySchema{
		ID: "s1", Type: domain.SurveySurvey, Title: "t", Version: 1,
		Questions: []domain.Question{
			{ID: "q1", Type: "single-choice", Required: true, Props: rawProps(map[string]any{
				"options": []map[string]string{{"value": "yes", "label": "用过"}, {"value": "no", "label": "没用过"}},
			})},
			{ID: "q2", Type: "scale", Required: true, Props: rawProps(map[string]any{"min": 1, "max": 5})},
		},
		Rules: []domain.LogicRule{
			{ID: "r1", Conditions: []domain.Condition{{QID: "q1", Op: domain.OpEq, Value: "no"}}, Combinator: domain.CombAnd, Action: domain.RuleAction{Type: domain.ActionHide, Target: "q2"}},
		},
	}
}

func TestValidateSurvey_HiddenSkipped(t *testing.T) {
	s := demoSchema()
	// 选 no → q2 隐藏 → 即使 q2 未答也不报必答
	errs := domain.ValidateSurvey(s, domain.Answers{"q1": "no"})
	if len(errs) != 0 {
		t.Fatalf("隐藏的必答题不应报错,得 %+v", errs)
	}
}

func TestValidateSurvey_RequiredVisible(t *testing.T) {
	s := demoSchema()
	// 选 yes → q2 可见且必答且未答 → 报必答
	errs := domain.ValidateSurvey(s, domain.Answers{"q1": "yes"})
	if len(errs) != 1 || errs[0].QID != "q2" || errs[0].Message != "此题为必答" {
		t.Fatalf("可见必答未答应报必答,得 %+v", errs)
	}
}

func TestValidateSurvey_DelegatesToHandler(t *testing.T) {
	s := demoSchema()
	// q2 越界 → 委托 scale handler 报错
	errs := domain.ValidateSurvey(s, domain.Answers{"q1": "yes", "q2": 9.0})
	if len(errs) != 1 || errs[0].QID != "q2" {
		t.Fatalf("应委托 handler 报刻度错,得 %+v", errs)
	}
}

func TestNormalizeSurvey_HiddenNotEmitted(t *testing.T) {
	s := demoSchema()
	// 选 no,q2 隐藏;即便客户端多传 q2 答案,也不该产 q2 行(不信任客户端)
	rows := domain.NormalizeSurvey(s, domain.Answers{"q1": "no", "q2": 5.0})
	for _, r := range rows {
		if r.QID == "q2" {
			t.Fatalf("隐藏题 q2 不应产规范化行,得 %+v", rows)
		}
	}
	if len(rows) != 1 || rows[0].QID != "q1" || rows[0].Value != "no" {
		t.Fatalf("应只产 q1 一行,得 %+v", rows)
	}
}

func TestNormalizeSurvey_Visible(t *testing.T) {
	s := demoSchema()
	rows := domain.NormalizeSurvey(s, domain.Answers{"q1": "yes", "q2": 4.0})
	if len(rows) != 2 {
		t.Fatalf("q1+q2 应产 2 行,得 %+v", rows)
	}
}
