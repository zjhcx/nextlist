package utils

import (
	"encoding/base64"
	"strings"

	"github.com/OpenListTeam/OpenList/v4/internal/conf"
)

func MappingName(name string) string {
	for k, v := range conf.FilenameCharMap {
		name = strings.ReplaceAll(name, k, v)
	}
	return name
}

var safeAtobReplacer = strings.NewReplacer("-", "+", "_", "/", ".", "=")

func SafeAtob(data string) (string, error) {
	bytes, err := base64.StdEncoding.DecodeString(safeAtobReplacer.Replace(data))
	if err != nil {
		return "", err
	}
	return string(bytes), err
}

// GetNoneEmpty returns the first non-empty string, return empty if all empty
func GetNoneEmpty(strArr ...string) string {
	for _, s := range strArr {
		if len(s) > 0 {
			return s
		}
	}
	return ""
}
