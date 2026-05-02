package server

import "strings"

const defaultLineName = "默认"

// normalizeLineName 将线路名称归一化，确保默认线路不为空。
func normalizeLineName(raw string) string {
	line := strings.TrimSpace(raw)
	if line == "" {
		return defaultLineName
	}
	lower := strings.ToLower(line)
	switch lower {
	case "default", "global", "all", "any":
		return defaultLineName
	}
	switch line {
	case "全网", "全球":
		return defaultLineName
	case "国内", "境内":
		return "境内"
	case "海外", "境外":
		return "境外"
	default:
		return line
	}
}

// normalizeLineKey 用于生成稳定的线路 key（默认线路兜底）。
func normalizeLineKey(raw string) string {
	return normalizeLineName(raw)
}
