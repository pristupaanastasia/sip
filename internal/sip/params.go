package sip

import "strings"

func Param(value, name string) string {
	if end := strings.LastIndexByte(value, '>'); end >= 0 {
		value = value[end+1:]
	}

	params := strings.Split(value, ";")
	for _, param := range params[1:] {
		key, paramValue, _ := strings.Cut(strings.TrimSpace(param), "=")
		if strings.EqualFold(strings.TrimSpace(key), name) {
			return strings.TrimSpace(paramValue)
		}
	}
	return ""
}
