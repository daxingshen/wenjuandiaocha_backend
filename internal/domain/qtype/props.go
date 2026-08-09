// 题型 props 解析辅助。props 是 json.RawMessage,各 handler 按自己的结构解出。
package qtype

import "encoding/json"

// option 选项 { value, label }。choice / matrix 列共用。
type option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// row 矩阵子行 { id, label }。
type row struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// unmarshalProps 把 Question.Props 解成目标结构;失败(props 为空/损坏)时目标保持零值。
func unmarshalProps(raw json.RawMessage, dst any) {
	if len(raw) == 0 {
		return
	}
	_ = json.Unmarshal(raw, dst)
}
