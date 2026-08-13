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
