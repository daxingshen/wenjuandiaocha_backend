package render

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"wenjuandiaocha_backend/internal/domain"
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

func TestSuccess_WrapsInEnvelope(t *testing.T) {
	w := run(func(c *gin.Context) { Success(c, gin.H{"id": "a"}) })
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

func TestSuccess_NilData(t *testing.T) {
	w := run(func(c *gin.Context) { Success(c, nil) })
	if w.Body.String() != `{"code":0,"message":"","data":null}` {
		t.Fatalf("body = %s, want data:null", w.Body.String())
	}
}

// 裸 schema 端点用 json.RawMessage 嵌入 data,原样输出不转义成字符串。
func TestSuccess_RawMessageNotEscaped(t *testing.T) {
	schema := json.RawMessage(`{"id":"s1","title":"问卷"}`)
	w := run(func(c *gin.Context) { Success(c, schema) })
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

func TestError_MapsEcodeToEnvelope(t *testing.T) {
	w := run(func(c *gin.Context) { Error(c, ecode.Conflict("仅进行中的问卷可结束")) })
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

func TestError_NonEcodeIsInternal(t *testing.T) {
	w := run(func(c *gin.Context) { Error(c, errStub{}) })
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

func TestValidation_ErrorsInData(t *testing.T) {
	errs := []domain.ValidationError{{QID: "q1", Message: "必答"}}
	w := run(func(c *gin.Context) { Validation(c, errs) })
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got struct {
		Code int `json:"code"`
		Data struct {
			Errors []domain.ValidationError `json:"errors"`
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
