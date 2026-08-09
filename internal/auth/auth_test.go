package auth

import "testing"

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("s3cret")
	if err != nil {
		t.Fatalf("hash 失败:%v", err)
	}
	if !CheckPassword(hash, "s3cret") {
		t.Error("正确密码应校验通过")
	}
	if CheckPassword(hash, "wrong") {
		t.Error("错误密码应校验失败")
	}
}

func TestSessionTokenUnique(t *testing.T) {
	a, err := NewSessionToken()
	if err != nil {
		t.Fatalf("gen token 失败:%v", err)
	}
	b, _ := NewSessionToken()
	if a == b {
		t.Error("两次生成的 token 不应相同")
	}
	if len(a) < 40 {
		t.Errorf("token 太短:%d", len(a))
	}
}
