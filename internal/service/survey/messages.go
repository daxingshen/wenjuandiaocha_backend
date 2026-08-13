package survey

// 问卷生命周期守卫的对外文案,集中一处管理(避免散落在各方法里、便于统一措辞与后续 i18n)。
// 均为状态机非法跳转时随 ecode.Conflict 返回给用户的说明。
const (
	msgEditForbidden = "已发布的问卷不可编辑"        // Update:仅 draft 可改
	msgCloseNotLive  = "仅进行中的问卷可结束"        // Close:仅 live 可结束
	msgReopenInvalid = "仅已结束且曾发布过的问卷可重新打开" // Reopen:仅 closed 且曾发布
)
