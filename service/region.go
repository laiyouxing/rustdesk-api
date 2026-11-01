package service

// 中国省级行政区划（34 个）。
//
// 用途：客户端/后台选择"设备所属地域"，用于中继就近调度。
// Code 用两位拼音首字母或国标缩写，稳定且短，存库/传参都用 Code；
// Name 仅用于展示。
//
// 说明：地域只是**排序偏好**，最终仍由客户端实测延迟决定（A+B 累加择路），
// 所以 Code 取值不要求与节点 region 完全一致——不一致时自然退化为纯延迟。
var ChinaProvinces = []map[string]string{
	{"code": "BJ", "name": "北京"},
	{"code": "TJ", "name": "天津"},
	{"code": "HE", "name": "河北"},
	{"code": "SX", "name": "山西"},
	{"code": "NM", "name": "内蒙古"},
	{"code": "LN", "name": "辽宁"},
	{"code": "JL", "name": "吉林"},
	{"code": "HL", "name": "黑龙江"},
	{"code": "SH", "name": "上海"},
	{"code": "JS", "name": "江苏"},
	{"code": "ZJ", "name": "浙江"},
	{"code": "AH", "name": "安徽"},
	{"code": "FJ", "name": "福建"},
	{"code": "JX", "name": "江西"},
	{"code": "SD", "name": "山东"},
	{"code": "HA", "name": "河南"},
	{"code": "HB", "name": "湖北"},
	{"code": "HN", "name": "湖南"},
	{"code": "GD", "name": "广东"},
	{"code": "GX", "name": "广西"},
	{"code": "HI", "name": "海南"},
	{"code": "CQ", "name": "重庆"},
	{"code": "SC", "name": "四川"},
	{"code": "GZ", "name": "贵州"},
	{"code": "YN", "name": "云南"},
	{"code": "XZ", "name": "西藏"},
	{"code": "SN", "name": "陕西"},
	{"code": "GS", "name": "甘肃"},
	{"code": "QH", "name": "青海"},
	{"code": "NX", "name": "宁夏"},
	{"code": "XJ", "name": "新疆"},
	{"code": "TW", "name": "台湾"},
	{"code": "HK", "name": "香港"},
	{"code": "MO", "name": "澳门"},
}

// ValidProvinceCode 校验地域代码是否合法（空串表示未设置，视为合法）。
func ValidProvinceCode(code string) bool {
	if code == "" {
		return true
	}
	for _, p := range ChinaProvinces {
		if p["code"] == code {
			return true
		}
	}
	return false
}
