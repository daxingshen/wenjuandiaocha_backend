// 黄金向量测试 —— 决策 6 的后端半边。读**前端那份** golden-vectors.json(单一真相源,
// 不复制进本仓),用后端 Evaluate 跑一遍,expectHidden 集合相等即证行为不漂移。
//
// 若前端仓不在隔壁(独立 CI),此测试会 skip 并提示——避免误判为通过。
package domain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// 前端黄金向量文件(相对本包目录)。
const goldenRelPath = "../../../wenjuandiaocha_ui/packages/engine/src/__tests__/golden-vectors.json"

type goldenFile struct {
	Vectors []goldenVector `json:"vectors"`
}

type goldenVector struct {
	Name         string      `json:"name"`
	Rules        []LogicRule `json:"rules"`
	Answers      Answers     `json:"answers"`
	ExpectHidden []string    `json:"expectHidden"`
}

func TestGoldenVectors(t *testing.T) {
	path := filepath.Clean(goldenRelPath)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("找不到前端黄金向量 %s(前端仓需在隔壁):%v", path, err)
		return
	}

	var gf goldenFile
	if err := json.Unmarshal(data, &gf); err != nil {
		t.Fatalf("解析 golden-vectors.json 失败:%v", err)
	}
	if len(gf.Vectors) == 0 {
		t.Fatal("黄金向量为空,契约文件异常")
	}

	for _, v := range gf.Vectors {
		t.Run(v.Name, func(t *testing.T) {
			got := Evaluate(v.Rules, v.Answers)
			gotSet := got.Hidden

			// 期望集合
			want := map[string]bool{}
			for _, id := range v.ExpectHidden {
				want[id] = true
			}

			if len(gotSet) != len(want) {
				t.Fatalf("hidden 数量不符:got %v want %v", sortedKeys(gotSet), v.ExpectHidden)
			}
			for id := range want {
				if !gotSet[id] {
					t.Fatalf("期望隐藏 %s 但未隐藏:got %v want %v", id, sortedKeys(gotSet), v.ExpectHidden)
				}
			}
		})
	}

	t.Logf("黄金向量全绿:%d 条", len(gf.Vectors))
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
