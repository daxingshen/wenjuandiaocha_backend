package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"wenjuandiaocha_backend/api"
	"wenjuandiaocha_backend/internal/ecode"
	"wenjuandiaocha_backend/internal/rbac"
)

func init() { gin.SetMode(gin.TestMode) }

// fakeAuth 是 auth.Service 的测试替身:ValidateSession 恒返回预设 role/userID(模拟有效会话)。
type fakeAuth struct{ role, userID string }

func (f fakeAuth) Login(context.Context, api.AuthLoginReq) (api.AuthLoginResp, error) {
	return api.AuthLoginResp{}, nil
}
func (f fakeAuth) Me(context.Context) (api.AuthMeResp, error) { return api.AuthMeResp{}, nil }
func (f fakeAuth) Logout(context.Context) error              { return nil }
func (f fakeAuth) ValidateSession(context.Context) (api.AuthValidateSessionResp, error) {
	return api.AuthValidateSessionResp{UserID: f.userID, Role: f.role}, nil
}

// runReq 起一个挂了 RequireAuth(svc, action) 的最小路由,带有效 sid cookie 回放请求。
func runReq(role string, action rbac.Action) *httptest.ResponseRecorder {
	r := gin.New()
	r.GET("/x", RequireAuth(fakeAuth{role: role, userID: "u_" + role}, action), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: "valid-token"})
	r.ServeHTTP(w, req)
	return w
}

// bizCode 从统一信封取业务 code。
func bizCode(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	var env struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("body not envelope json: %v (%s)", err, w.Body.String())
	}
	return env.Code
}

// respondent 调创作端能力位(如 survey:read)→ 403 能力级越权,不进 handler。
func TestRequireAuth_Respondent_CreatorAction_Forbidden403(t *testing.T) {
	for _, action := range []rbac.Action{
		rbac.ActionSurveyList, rbac.ActionSurveyRead, rbac.ActionSurveyCreate,
		rbac.ActionSurveyUpdate, rbac.ActionSurveyPublish, rbac.ActionSurveyClose,
		rbac.ActionSurveyReopen, rbac.ActionSurveyStats,
	} {
		w := runReq("respondent", action)
		if w.Code != http.StatusOK { // 统一信封恒 200 HTTP
			t.Fatalf("action=%s HTTP=%d, want 200 (业务错走信封)", action, w.Code)
		}
		if got := bizCode(t, w); got != ecode.CodeForbidden {
			t.Fatalf("respondent action=%s biz code=%d, want CodeForbidden(403)", action, got)
		}
	}
}

// creator 调创作端能力位 → 通过(进 handler),能力位不拦(归属另在 service 判)。
func TestRequireAuth_Creator_CreatorAction_Passes(t *testing.T) {
	w := runReq("creator", rbac.ActionSurveyUpdate)
	if w.Code != http.StatusOK || w.Body.String() != `{"ok":true}` {
		t.Fatalf("creator 能力位应通过: HTTP=%d body=%s", w.Code, w.Body.String())
	}
}

// admin 调任意创作端能力位 → 通过。
func TestRequireAuth_Admin_Passes(t *testing.T) {
	w := runReq("admin", rbac.ActionSurveyPublish)
	if w.Code != http.StatusOK || w.Body.String() != `{"ok":true}` {
		t.Fatalf("admin 能力位应通过: HTTP=%d body=%s", w.Code, w.Body.String())
	}
}

// action=="" → 只鉴权不判能力位:respondent(创作端无能力)也放行(如 /auth/me、submit 登录路由)。
func TestRequireAuth_EmptyAction_SkipsCapability(t *testing.T) {
	w := runReq("respondent", "")
	if w.Code != http.StatusOK || w.Body.String() != `{"ok":true}` {
		t.Fatalf("action='' 应只鉴权放行: HTTP=%d body=%s", w.Code, w.Body.String())
	}
}

// 无 cookie → 401 未登录(原生 HTTP 状态,不进信封),能力位判定之前先拦。
func TestRequireAuth_NoCookie_Unauthorized(t *testing.T) {
	r := gin.New()
	r.GET("/x", RequireAuth(fakeAuth{role: "admin"}, rbac.ActionSurveyList), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("无 cookie HTTP=%d, want 401", w.Code)
	}
}
