package rbac

import "testing"

// 全矩阵逐格断言,对齐 wiki/RBAC-账号权限设计.md「能力矩阵」表。
// want[role][action] = 期望值;表驱动确保加角色/改能力时测试同步暴露差异。
func TestCan_FullMatrix(t *testing.T) {
	allActions := []Action{
		ActionSurveyList, ActionSurveyRead, ActionSurveyStats, ActionSurveyCreate,
		ActionSurveyUpdate, ActionSurveyPublish, ActionSurveyClose, ActionSurveyReopen,
		ActionSubmitAnswer, ActionAccountManage,
	}

	// creator 的创作端 8 动作全 ✓,作答/账号管理 ✗。
	creatorWant := map[Action]bool{
		ActionSurveyList: true, ActionSurveyRead: true, ActionSurveyStats: true,
		ActionSurveyCreate: true, ActionSurveyUpdate: true, ActionSurveyPublish: true,
		ActionSurveyClose: true, ActionSurveyReopen: true,
		ActionSubmitAnswer: false, ActionAccountManage: false,
	}
	// respondent 仅作答 ✓,其余全 ✗。
	respondentWant := map[Action]bool{
		ActionSurveyList: false, ActionSurveyRead: false, ActionSurveyStats: false,
		ActionSurveyCreate: false, ActionSurveyUpdate: false, ActionSurveyPublish: false,
		ActionSurveyClose: false, ActionSurveyReopen: false,
		ActionSubmitAnswer: true, ActionAccountManage: false,
	}

	for _, a := range allActions {
		// admin 全 ✓(超级权限能做一切)。
		if !Can(RoleAdmin, a) {
			t.Errorf("admin 应可 %q", a)
		}
		if got := Can(RoleCreator, a); got != creatorWant[a] {
			t.Errorf("creator %q = %v, want %v", a, got, creatorWant[a])
		}
		if got := Can(RoleRespondent, a); got != respondentWant[a] {
			t.Errorf("respondent %q = %v, want %v", a, got, respondentWant[a])
		}
	}
}

// 关键区分:creator 不能作答,respondent/admin 能 —— login_required 作答门控的核心。
func TestCan_SubmitAnswer_RoleSplit(t *testing.T) {
	if Can(RoleCreator, ActionSubmitAnswer) {
		t.Error("creator 不应能作答提交")
	}
	if !Can(RoleRespondent, ActionSubmitAnswer) {
		t.Error("respondent 应能作答提交")
	}
	if !Can(RoleAdmin, ActionSubmitAnswer) {
		t.Error("admin 应能作答提交")
	}
}

// respondent 创作端一律拒 —— 它存在的唯一意义就是作答。
func TestCan_Respondent_StudioDenied(t *testing.T) {
	studio := []Action{
		ActionSurveyList, ActionSurveyRead, ActionSurveyStats, ActionSurveyCreate,
		ActionSurveyUpdate, ActionSurveyPublish, ActionSurveyClose, ActionSurveyReopen,
	}
	for _, a := range studio {
		if Can(RoleRespondent, a) {
			t.Errorf("respondent 不应能创作端动作 %q", a)
		}
	}
}

// 账号管理仅 admin —— 平台能力级越权(非 admin 调)由此挡下(service 层据此返 403)。
func TestCan_AccountManage_AdminOnly(t *testing.T) {
	if !Can(RoleAdmin, ActionAccountManage) {
		t.Error("admin 应可账号管理")
	}
	if Can(RoleCreator, ActionAccountManage) || Can(RoleRespondent, ActionAccountManage) {
		t.Error("非 admin 不应能账号管理")
	}
}

// 未知角色 / 空角色一律无能力(默认拒绝)。
func TestCan_UnknownRole_DeniesAll(t *testing.T) {
	if Can(Role("ghost"), ActionSurveyList) || Can(Role(""), ActionSubmitAnswer) {
		t.Error("未知/空角色应无任何能力")
	}
}

func TestIsAdmin(t *testing.T) {
	if !IsAdmin(RoleAdmin) {
		t.Error("admin 应 IsAdmin=true")
	}
	if IsAdmin(RoleCreator) || IsAdmin(RoleRespondent) || IsAdmin(Role("ghost")) {
		t.Error("非 admin 应 IsAdmin=false")
	}
}
