// Package rbac 是账号角色的能力判定层:一张静态「角色 × 动作」能力矩阵 can()。
//
// 这是两层授权模型的第一层(平台角色能否做这类动作),与具体资源无关;第二层
// 「资源归属」仍由 service 的 owned() 承担。判定顺序固定:先 can() 后 owned(),
// admin 短路 owned()。设计与矩阵权威见 wiki/RBAC-账号权限设计.md「能力矩阵」节。
//
// 角色少、变更需 code review,故矩阵硬编码在此,不做运行时可配置(方案 A)。
// role 决定「身份能对谁的资源做什么」。
package rbac

// Role 是账号角色(users.role)。零值不是合法角色 —— 未知角色一律无能力。
type Role string

const (
	RoleAdmin      Role = "admin"      // 平台超级权限:跨 owner 执行全部 creator 动作 + 账号管理 + 可作答
	RoleCreator    Role = "creator"    // 默认角色:只管自己的问卷(建/改/发/收);不可作答
	RoleRespondent Role = "respondent" // 只能作答:凭链接对 login_required 问卷提交;创作端一律拒
)

// Action 是受控动作。动作全集 = 当前 studio 端点 + 作答提交 + 账号管理。
type Action string

const (
	ActionSurveyList    Action = "survey:list"    // 列问卷(admin 全站 / creator 仅本人,范围差异在 service)
	ActionSurveyRead    Action = "survey:read"    // 读草稿(创作端)
	ActionSurveyStats   Action = "survey:stats"   // 看回收统计
	ActionSurveyCreate  Action = "survey:create"  // 新建
	ActionSurveyUpdate  Action = "survey:update"  // 改草稿
	ActionSurveyPublish Action = "survey:publish" // 发布
	ActionSurveyClose   Action = "survey:close"   // 结束
	ActionSurveyReopen  Action = "survey:reopen"  // 重开
	ActionSubmitAnswer  Action = "answer:submit"  // 作答提交(仅 login_required 问卷经鉴权路径)
	ActionAccountManage Action = "account:manage" // 账号管理(改角色/封禁等;本轮只留能力位,无端点)
)

// matrix 是能力矩阵:matrix[role][action]==true 表示该角色可做该类动作(仍受第二层归属约束)。
// 缺省 false —— 未列的组合一律拒绝。与 wiki「能力矩阵」表逐格一致。
var matrix = map[Role]map[Action]bool{
	RoleAdmin: {
		ActionSurveyList:    true,
		ActionSurveyRead:    true,
		ActionSurveyStats:   true,
		ActionSurveyCreate:  true,
		ActionSurveyUpdate:  true,
		ActionSurveyPublish: true,
		ActionSurveyClose:   true,
		ActionSurveyReopen:  true,
		ActionSubmitAnswer:  true, // 超级权限保留作答能力
		ActionAccountManage: true,
	},
	RoleCreator: {
		ActionSurveyList:    true,
		ActionSurveyRead:    true,
		ActionSurveyStats:   true,
		ActionSurveyCreate:  true,
		ActionSurveyUpdate:  true,
		ActionSurveyPublish: true,
		ActionSurveyClose:   true,
		ActionSurveyReopen:  true,
		// ActionSubmitAnswer 缺省 false:creator 是创作身份,作答是独立身份,不混入作答数据
		// ActionAccountManage 缺省 false
	},
	RoleRespondent: {
		ActionSubmitAnswer: true, // 唯一能力:作答提交。创作端动作全缺省 false
	},
}

// Can 判定 role 是否可执行 action(第一层平台能力位)。未知 role / 未列 action 一律 false。
func Can(role Role, action Action) bool {
	return matrix[role][action]
}

// IsAdmin 判定是否平台超级权限(admin 短路第二层资源归属)。
func IsAdmin(role Role) bool {
	return role == RoleAdmin
}
