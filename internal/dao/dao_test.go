// schemaEqualIgnoringVersion 的单测:重发免空版的判等核心。
// 假阴性(草稿变了却判 equal)= 静默吞掉真实编辑,是最严重的坑,故逐个反例锁死。
package dao

import "testing"

func TestSchemaEqualIgnoringVersion(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{
			name: "仅 version 不同 → 相等(发布必改写 version,属设计性差异)",
			a:    `{"id":"s1","title":"t","version":1,"questions":[{"id":"q1","title":"Q1"}],"rules":[]}`,
			b:    `{"id":"s1","title":"t","version":7,"questions":[{"id":"q1","title":"Q1"}],"rules":[]}`,
			want: true,
		},
		{
			name: "字节序/键序/空白不同但内容同 → 相等(map 判等,与序无关)",
			a:    `{"version":1,"title":"t","id":"s1","questions":[{"title":"Q1","id":"q1"}],"rules":[]}`,
			b:    `{  "id":"s1", "title":"t", "version":2, "questions":[ {"id":"q1","title":"Q1"} ], "rules":[] }`,
			want: true,
		},
		{
			name: "新增题目 → 有变",
			a:    `{"id":"s1","version":1,"questions":[{"id":"q1","title":"Q1"}]}`,
			b:    `{"id":"s1","version":2,"questions":[{"id":"q1","title":"Q1"},{"id":"q2","title":"Q2"}]}`,
			want: false,
		},
		{
			name: "题目改序 → 有变(数组按序判等)",
			a:    `{"id":"s1","version":1,"questions":[{"id":"q1"},{"id":"q2"}]}`,
			b:    `{"id":"s1","version":2,"questions":[{"id":"q2"},{"id":"q1"}]}`,
			want: false,
		},
		{
			name: "改标题 → 有变",
			a:    `{"id":"s1","version":1,"questions":[{"id":"q1","title":"旧"}]}`,
			b:    `{"id":"s1","version":2,"questions":[{"id":"q1","title":"新"}]}`,
			want: false,
		},
		{
			name: "改规则 → 有变",
			a:    `{"id":"s1","version":1,"rules":[]}`,
			b:    `{"id":"s1","version":2,"rules":[{"id":"r1","action":{"type":"hide","target":"q2"}}]}`,
			want: false,
		},
		{
			name: "删题目字段 → 有变(delete 只作用顶层 version,不误删题内字段)",
			a:    `{"id":"s1","version":1,"questions":[{"id":"q1","required":true}]}`,
			b:    `{"id":"s1","version":2,"questions":[{"id":"q1"}]}`,
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := schemaEqualIgnoringVersion([]byte(tc.a), []byte(tc.b))
			if err != nil {
				t.Fatalf("意外错误: %v", err)
			}
			if got != tc.want {
				t.Errorf("判等 = %v, 期望 %v", got, tc.want)
			}
		})
	}
}

func TestSchemaEqualIgnoringVersion_BadJSON(t *testing.T) {
	if _, err := schemaEqualIgnoringVersion([]byte(`{bad`), []byte(`{}`)); err == nil {
		t.Error("非法 JSON 应返回错误")
	}
}
