// 中国省级行政区名单(34 个)。复刻前端 packages/question-types/src/shared/provinces.ts,
// 两份人工同步,各带 len==34 哨兵测试。供「属性验证=省份」的填空/多项填空校验。
package qtype

// provinceList 34 个省级行政区(4 直辖市 + 23 省 + 5 自治区 + 2 特别行政区)。
var provinceList = []string{
	// 直辖市 4
	"北京市", "天津市", "上海市", "重庆市",
	// 省 23
	"河北省", "山西省", "辽宁省", "吉林省", "黑龙江省",
	"江苏省", "浙江省", "安徽省", "福建省", "江西省",
	"山东省", "河南省", "湖北省", "湖南省", "广东省",
	"海南省", "四川省", "贵州省", "云南省", "陕西省",
	"甘肃省", "青海省", "台湾省",
	// 自治区 5
	"内蒙古自治区", "广西壮族自治区", "西藏自治区", "宁夏回族自治区", "新疆维吾尔自治区",
	// 特别行政区 2
	"香港特别行政区", "澳门特别行政区",
}

// provinceSet 名单集合,供 O(1) 校验。
var provinceSet = func() map[string]bool {
	m := make(map[string]bool, len(provinceList))
	for _, p := range provinceList {
		m[p] = true
	}
	return m
}()
