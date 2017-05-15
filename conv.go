package jmespath

import (
	"encoding/json"
)

func convToString(inp interface{}) (string, error) {
	if v, ok := inp.(string); ok {
		return v, nil
	}
	result, err := json.Marshal(inp)
	if err != nil {
		return "", err
	}
	return string(result), nil
}
