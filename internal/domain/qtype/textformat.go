// 文本题共享校验:11 项属性验证 format + 通用 validateTextValue。
// 复刻前端 packages/question-types/src/shared/text-format.ts,正则/边界逐字对齐。
// 填空(text-input)、多项填空(multi-fill 每框)共用。maxLength/minLength 用 utf16Len(对齐 JS length)。
package qtype

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// 正则:与前端 text-format.ts 完全一致。
var (
	integerRE = regexp.MustCompile(`^-?\d+$`)
	decimalRE = regexp.MustCompile(`^-?\d+(\.\d+)?$`)
	dateRE    = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	ageRE     = regexp.MustCompile(`^\d+$`)
	zipcodeRE = regexp.MustCompile(`^\d{6}$`)
	urlRE     = regexp.MustCompile(`^https?://\S+$`)
	idcardRE  = regexp.MustCompile(`^\d{17}[\dXx]$`)
)

// normalizeFormat 非法/缺省一律 text。
func normalizeFormat(f string) string {
	switch f {
	case "email", "phone", "integer", "decimal", "date", "age", "province", "idcard", "zipcode", "url":
		return f
	default:
		return "text"
	}
}

// isValidDate 正则过后再查合法日历日(YYYY-MM-DD 且年月日相符)。
func isValidDate(s string) bool {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return false
	}
	return t.Format("2006-01-02") == s
}

// isValidIdcard 18 位 + mod-11-2 校验码(GB 11643-1999),与前端一致。
func isValidIdcard(s string) bool {
	if !idcardRE.MatchString(s) {
		return false
	}
	weights := []int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}
	checks := []byte{'1', '0', 'X', '9', '8', '7', '6', '5', '4', '3', '2'}
	sum := 0
	for i := 0; i < 17; i++ {
		sum += int(s[i]-'0') * weights[i]
	}
	return checks[sum%11] == byte(strings.ToUpper(string(s[17]))[0])
}

// checkFormat 属性验证不符时返回错误文案;format=text 恒空。
func checkFormat(value, format string) string {
	switch format {
	case "email":
		if !emailRE.MatchString(value) {
			return "邮箱格式不正确"
		}
	case "phone":
		if !phoneRE.MatchString(value) {
			return "手机号格式不正确"
		}
	case "integer":
		if !integerRE.MatchString(value) {
			return "请填写整数"
		}
	case "decimal":
		if !decimalRE.MatchString(value) {
			return "请填写数字"
		}
	case "date":
		if !dateRE.MatchString(value) || !isValidDate(value) {
			return "日期格式应为 YYYY-MM-DD"
		}
	case "age":
		n, err := strconv.Atoi(value)
		if !ageRE.MatchString(value) || err != nil || n < 0 || n > 150 {
			return "年龄应为 0–150 的整数"
		}
	case "province":
		if !provinceSet[value] {
			return "请选择省份"
		}
	case "idcard":
		if !isValidIdcard(value) {
			return "身份证号格式不正确"
		}
	case "zipcode":
		if !zipcodeRE.MatchString(value) {
			return "邮编应为 6 位数字"
		}
	case "url":
		if !urlRE.MatchString(value) {
			return "网址格式不正确"
		}
	}
	return ""
}

// textRules 文本框校验配置(填空整题 / 多项填空每框共用)。
type textRules struct {
	Format    string
	MinLength *int
	MaxLength *int
}

// validateTextValue 校验一个文本值。返回错误串或 ""(通过)。
// 顺序:类型 → 最多字数 → 最少字数 → 属性验证。空串是否豁免由调用方决定。
func validateTextValue(answer any, r textRules) string {
	s, ok := answer.(string)
	if !ok {
		return "答案格式应为文本"
	}
	if r.MaxLength != nil && utf16Len(s) > *r.MaxLength {
		return fmt.Sprintf("不超过 %d 个字符", *r.MaxLength)
	}
	if r.MinLength != nil && utf16Len(s) < *r.MinLength {
		return fmt.Sprintf("至少 %d 个字符", *r.MinLength)
	}
	return checkFormat(s, normalizeFormat(r.Format))
}
