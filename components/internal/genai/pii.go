package genai

import "regexp"

// DefaultScrubPatterns returns regexes for common PII / secret shapes that the
// redaction processor masks out of string attribute values by default. These
// are best-effort heuristics, not a substitute for a real DLP system — see
// docs/adr/0004 for the rationale and limitations.
func DefaultScrubPatterns() []string {
	return []string{
		`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`,                         // email address
		`\b\d{4}[ -]\d{4}[ -]\d{4}[ -]\d{4}\b`,                                     // grouped 16-digit card
		`\b\d{15,16}\b`,                                                            // bare 15-16 digit card
		`\b\d{3}[- ]\d{2}[- ]\d{4}\b`,                                              // US SSN (separated)
		`(?i)\b(?:sk|pk|api[_-]?key|secret|token|bearer)[-_ :=]+[a-z0-9._\-]{12,}`, // keyword-prefixed secrets
		`\bAKIA[0-9A-Z]{16}\b`,                                                     // AWS access key id
		`\bghp_[A-Za-z0-9]{36}\b`,                                                  // GitHub personal access token
		`\bAIza[0-9A-Za-z_\-]{35}\b`,                                               // Google API key
		`\beyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+`,                   // JWT
	}
}

// Scrubber masks substrings matching any of its compiled patterns.
type Scrubber struct {
	res  []*regexp.Regexp
	mask string
}

// NewScrubber compiles the given patterns once. A compile error is returned so
// configuration can be validated up front.
func NewScrubber(patterns []string, mask string) (*Scrubber, error) {
	res := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, err
		}
		res = append(res, re)
	}
	if mask == "" {
		mask = "[REDACTED]"
	}
	return &Scrubber{res: res, mask: mask}, nil
}

// Scrub replaces all pattern matches in s with the mask and reports whether any
// replacement happened.
func (s *Scrubber) Scrub(in string) (out string, changed bool) {
	out = in
	for _, re := range s.res {
		out = re.ReplaceAllString(out, s.mask)
	}
	return out, out != in
}
