package domain

// 问卷生命周期状态。区别于本包其余「对齐前端 engine schema」的类型:
// status 不在 engine SurveySchema 里,是后端权威的生命周期字段(studio 列表/概览
// 作为 wire 字段透出,存于 surveys.status 列)。状态机:draft→live(publish)→
// closed(close)→live(reopen)。集中定义,避免各处散字符串漂移。
//
// 用无类型字符串常量(而非具名类型),以便与 dao.SurveyMeta.Status(string)、
// Store.SetStatus(...string) 直接比较/传参,无需转换。
const (
	StatusDraft  = "draft"  // 草稿:仅此态可编辑内容
	StatusLive   = "live"   // 进行中:已发布、正在回收答卷
	StatusClosed = "closed" // 已截止:暂停回收(曾发布过可 reopen)
)

// 问卷作答访问模式(surveys.answer_access 列,发布时设定)。同为后端权威的问卷级配置,
// 决定谁能提交答卷:anonymous 免登录走 /public;login_required 需登录且有作答能力(respondent/admin)。
// 与生命周期 status 正交。集中定义避免各处散字符串漂移。
const (
	AnswerAnonymous     = "anonymous"      // 匿名作答(默认,维持现状):任何人免登录,走 /api/public
	AnswerLoginRequired = "login_required" // 需登录作答:过 requireAuth + 作答能力位,走鉴权提交端点
)

// 问卷作答页展示模式(surveys.display_mode 列,draft 阶段设定)。同为后端权威的问卷级配置,
// 决定作答者填写页的呈现:paged 一屏一题、single 全部题目同屏。与生命周期 status 正交。
// 集中定义避免各处散字符串漂移。
const (
	DisplayPaged  = "paged"  // 分页作答:一屏一题(逐题翻页)
	DisplaySingle = "single" // 单页作答(默认):所有题目同屏展示
)
