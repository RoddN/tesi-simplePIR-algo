package utils

import (
	"encoding/json"
	"fmt"

	"github.com/ugorji/go/codec"
)

func SanitizeForJSON(v any) any {
	switch val := v.(type) {
	case map[any]any:
		cleanMap := make(map[string]any)
		for k, mapVal := range val {
			var strKey string
			switch keyVal := k.(type) {
			case string:
				strKey = keyVal
			case []byte:
				strKey = string(keyVal)
			default:
				strKey = fmt.Sprintf("%v", keyVal)
			}
			cleanMap[strKey] = SanitizeForJSON(mapVal)
		}
		return cleanMap
	case map[string]any:
		cleanMap := make(map[string]any)
		for k, mapVal := range val {
			cleanMap[k] = SanitizeForJSON(mapVal)
		}
		return cleanMap
	case []any:
		cleanSlice := make([]any, len(val))
		for i, item := range val {
			cleanSlice[i] = SanitizeForJSON(item)
		}
		return cleanSlice
	default:
		return val
	}
}

// DecodeMsgpackToJson riceve direttamente l'array di byte (es. dopo l'unpack del PIR)
func DecodeMsgpackToJson(b []byte) (string, error) {
	if len(b) == 0 {
		return "", fmt.Errorf("data is empty")
	}

	var decoded any
	var mh codec.MsgpackHandle

	dec := codec.NewDecoderBytes(b, &mh)
	if err := dec.Decode(&decoded); err != nil {
		return "", fmt.Errorf("msgpack decode error: %v", err)
	}

	cleanData := SanitizeForJSON(decoded)

	prettyOutput, err := json.MarshalIndent(cleanData, "", "  ")
	if err != nil {
		return "", fmt.Errorf("json format error: %v", err)
	}

	return string(prettyOutput), nil
}
