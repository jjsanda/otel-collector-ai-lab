package genai

import (
	"strconv"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

// IsGenAIAttrs reports whether an attribute set looks like a GenAI operation
// (it carries an operation name or a request model).
func IsGenAIAttrs(attrs pcommon.Map) bool {
	if _, ok := attrs.Get(AttrOperationName); ok {
		return true
	}
	_, ok := attrs.Get(AttrRequestModel)
	return ok
}

// StrAttr returns the string value of key, or "" if absent.
func StrAttr(attrs pcommon.Map, key string) string {
	if v, ok := attrs.Get(key); ok {
		return v.AsString()
	}
	return ""
}

// IntAttr returns the integer value of key, coercing from Int, Double, or a
// numeric string. Returns 0 if absent or unparseable.
func IntAttr(attrs pcommon.Map, key string) int64 {
	v, ok := attrs.Get(key)
	if !ok {
		return 0
	}
	switch v.Type() {
	case pcommon.ValueTypeInt:
		return v.Int()
	case pcommon.ValueTypeDouble:
		return int64(v.Double())
	case pcommon.ValueTypeStr:
		if n, err := strconv.ParseInt(v.Str(), 10, 64); err == nil {
			return n
		}
	}
	return 0
}
