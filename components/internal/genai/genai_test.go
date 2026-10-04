package genai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestEstimateCostUSD(t *testing.T) {
	p := DefaultPricing()
	cost, ok := EstimateCostUSD(p, "gpt-4o-mini", 1000, 2000)
	assert.True(t, ok)
	assert.InDelta(t, 0.00135, cost, 1e-9)

	_, ok = EstimateCostUSD(p, "unknown-model", 10, 10)
	assert.False(t, ok)
}

func TestScrubber(t *testing.T) {
	s, err := NewScrubber(DefaultScrubPatterns(), "[X]")
	assert.NoError(t, err)

	out, changed := s.Scrub("mail me at a@b.com")
	assert.True(t, changed)
	assert.Equal(t, "mail me at [X]", out)

	out, changed = s.Scrub("nothing sensitive here")
	assert.False(t, changed)
	assert.Equal(t, "nothing sensitive here", out)
}

func TestScrubber_BadPattern(t *testing.T) {
	_, err := NewScrubber([]string{"("}, "[X]")
	assert.Error(t, err)
}

func TestScrubber_SecretShapes(t *testing.T) {
	s, err := NewScrubber(DefaultScrubPatterns(), "[X]")
	assert.NoError(t, err)
	cases := []string{
		"AKIAIOSFODNN7EXAMPLE",                                 // AWS access key id
		"ghp_1234567890123456789012345678901234ab",            // GitHub PAT
		"jwt eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.abcDEF_-123", // JWT
		"card 4111 1111 1111 1111",                             // grouped card number
		"ssn 123-45-6789",                                      // separated SSN
	}
	for _, in := range cases {
		out, changed := s.Scrub(in)
		assert.True(t, changed, "expected a scrub for %q", in)
		assert.Contains(t, out, "[X]")
	}
}

func TestAttrHelpers(t *testing.T) {
	m := pcommon.NewMap()
	m.PutStr("s", "hello")
	m.PutInt("i", 42)
	m.PutDouble("d", 7.9)
	m.PutStr("ns", "13")

	assert.Equal(t, "hello", StrAttr(m, "s"))
	assert.Equal(t, "", StrAttr(m, "missing"))
	assert.EqualValues(t, 42, IntAttr(m, "i"))
	assert.EqualValues(t, 7, IntAttr(m, "d"))
	assert.EqualValues(t, 13, IntAttr(m, "ns"))
	assert.EqualValues(t, 0, IntAttr(m, "missing"))
}

func TestIsGenAIAttrs(t *testing.T) {
	empty := pcommon.NewMap()
	assert.False(t, IsGenAIAttrs(empty))

	byOp := pcommon.NewMap()
	byOp.PutStr(AttrOperationName, "chat")
	assert.True(t, IsGenAIAttrs(byOp))

	byModel := pcommon.NewMap()
	byModel.PutStr(AttrRequestModel, "gpt-4o")
	assert.True(t, IsGenAIAttrs(byModel))
}

func TestSensitiveContentKeys(t *testing.T) {
	keys := SensitiveContentKeys()
	assert.Contains(t, keys, AttrInputMessages)
	assert.Contains(t, keys, AttrOutputMessages)
	assert.Contains(t, keys, AttrSystemInstructions)
}
