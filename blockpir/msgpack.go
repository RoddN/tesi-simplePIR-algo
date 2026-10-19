package blockpir

import (
	"encoding/json"
	"fmt"

	"github.com/ugorji/go/codec"
)

func MsgpackToJSON(b []byte) (string, error) {
	if len(b) == 0 {
		return "", fmt.Errorf("blocco vuoto")
	}

	var decoded any
	var mh codec.MsgpackHandle
	if err := codec.NewDecoderBytes(b, &mh).Decode(&decoded); err != nil {
		return "", fmt.Errorf("decodifica msgpack: %v", err)
	}

	out, err := json.MarshalIndent(toJSONValue(decoded), "", "  ")
	if err != nil {
		return "", fmt.Errorf("conversione in JSON: %v", err)
	}
	return string(out), nil
}

func toJSONValue(v any) any {
	switch val := v.(type) {
	case map[any]any:
		out := make(map[string]any)
		for k, item := range val {
			switch key := k.(type) {
			case string:
				out[key] = toJSONValue(item)
			case []byte:
				out[string(key)] = toJSONValue(item)
			default:
				out[fmt.Sprintf("%v", key)] = toJSONValue(item)
			}
		}
		return out
	case map[string]any:
		out := make(map[string]any)
		for k, item := range val {
			out[k] = toJSONValue(item)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = toJSONValue(item)
		}
		return out
	default:
		return val
	}
}
