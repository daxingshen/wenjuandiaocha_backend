package render

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"wenjuandiaocha_backend/api"
	"wenjuandiaocha_backend/internal/ecode"
)

func init() { gin.SetMode(gin.TestMode) }

// run 起一个只跑 handler 的 gin engine,回放请求拿响应。
func run(handler gin.HandlerFunc) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	handler(c)
	return w
}

// JSON(err==nil) 成功负载包进信封。
func TestJSON_SuccessWrapsInEnvelope(t *testing.T) {
	w := run(func(c *gin.Context) { JSON(c, gin.H{"id": "a"}, nil) })
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got struct {
		Code    int            `json:"code"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("body not json: %v (%s)", err, w.Body.String())
	}
	if got.Code != ecode.CodeOK || got.Message != "" || got.Data["id"] != "a" {
		t.Fatalf("envelope = %+v, want {code:0,message:'',data:{id:a}}", got)
	}
}

// JSON(nil, nil) 占位成功 → data:null。
func TestJSON_NilData(t *testing.T) {
	w := run(func(c *gin.Context) { JSON(c, nil, nil) })
	if w.Body.String() != `{"code":0,"message":"","data":null}` {
		t.Fatalf("body = %s, want data:null", w.Body.String())
	}
}

// 裸 schema 端点用 json.RawMessage 作 data,原样输出不转义成字符串。
func TestJSON_RawMessageNotEscaped(t *testing.T) {
	schema := json.RawMessage(`{"id":"s1","title":"问卷"}`)
	w := run(func(c *gin.Context) { JSON(c, schema, nil) })
	var got struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("body not json: %v (%s)", err, w.Body.String())
	}
	if got.Data["id"] != "s1" || got.Data["title"] != "问卷" {
		t.Fatalf("data = %+v, want schema object (not escaped string)", got.Data)
	}
}

// JSON(err=ecode.Error) 映射业务码,恒 200,data:null。
func TestJSON_EcodeErrorToEnvelope(t *testing.T) {
	w := run(func(c *gin.Context) { JSON(c, nil, ecode.Conflict("仅进行中的问卷可结束")) })
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (business error stays 200)", w.Code)
	}
	var got struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    any    `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Code != ecode.CodeConflict || got.Message != "仅进行中的问卷可结束" || got.Data != nil {
		t.Fatalf("envelope = %+v, want {code:40901,message:...,data:null}", got)
	}
}

// JSON(err=非 ecode.Error) 归 CodeInternal。
func TestJSON_NonEcodeIsInternal(t *testing.T) {
	w := run(func(c *gin.Context) { JSON(c, nil, errStub{}) })
	var got struct {
		Code int `json:"code"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Code != ecode.CodeInternal {
		t.Fatalf("code = %d, want CodeInternal", got.Code)
	}
}

type errStub struct{}

func (errStub) Error() string { return "boom" }

// JSON(err=ecode.Validation(payload)) 携带的 data 原样渲染进信封 → data:{errors:[...]}。
// 校验错现以 error 形式经 JSON 统一分派(不再有独立 Validation 出口)。
func TestJSON_ValidationErrorCarriesDataPayload(t *testing.T) {
	errs := []api.ValidationError{{QID: "q1", Message: "必答"}}
	verr := ecode.Validation(api.ValidationErrorsPayload{Errors: errs})
	w := run(func(c *gin.Context) { JSON(c, nil, verr) })
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got struct {
		Code int `json:"code"`
		Data struct {
			Errors []api.ValidationError `json:"errors"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("body not json: %v (%s)", err, w.Body.String())
	}
	if got.Code != ecode.CodeValidation || len(got.Data.Errors) != 1 || got.Data.Errors[0].QID != "q1" {
		t.Fatalf("envelope = %+v, want {code:42201,data:{errors:[{qid:q1}]}}", got)
	}
}

// Fail 保留原生非-200,不进信封(传输层失败)。
func TestFail_KeepsNativeStatus(t *testing.T) {
	w := run(func(c *gin.Context) { Fail(c, http.StatusTooManyRequests, "太频繁") })
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 (transport failure keeps native status)", w.Code)
	}
	if w.Body.String() != `{"error":"太频繁"}` {
		t.Fatalf("body = %s, want {error:...}", w.Body.String())
	}
}
