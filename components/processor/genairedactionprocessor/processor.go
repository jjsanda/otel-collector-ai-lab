// Package genairedactionprocessor sanitizes and enriches GenAI ("gen_ai.*")
// telemetry: it redacts prompt/response content, scrubs PII from remaining
// string values, normalizes the provider attribute, and derives an estimated
// request cost from token usage. It operates on both traces and logs.
package genairedactionprocessor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jjsanda/otel-collector-ai-lab/components/internal/genai"
)

const deploymentEnvironmentKey = "deployment.environment"

// redactor holds the compiled, ready-to-run sanitization state.
type redactor struct {
	cfg      *Config
	scrubber *genai.Scrubber
	redact   map[string]struct{}
	logger   *zap.Logger
}

func newRedactor(cfg *Config, logger *zap.Logger) (*redactor, error) {
	sc, err := genai.NewScrubber(cfg.ScrubPatterns, cfg.MaskString)
	if err != nil {
		return nil, err
	}
	rm := make(map[string]struct{}, len(cfg.RedactKeys))
	for _, k := range cfg.RedactKeys {
		rm[k] = struct{}{}
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &redactor{cfg: cfg, scrubber: sc, redact: rm, logger: logger}, nil
}

func (r *redactor) processTraces(_ context.Context, td ptrace.Traces) (ptrace.Traces, error) {
	rss := td.ResourceSpans()
	for i := 0; i < rss.Len(); i++ {
		rs := rss.At(i)
		r.enrichResource(rs.Resource().Attributes())
		sss := rs.ScopeSpans()
		for j := 0; j < sss.Len(); j++ {
			spans := sss.At(j).Spans()
			for k := 0; k < spans.Len(); k++ {
				r.sanitizeSpan(spans.At(k))
			}
		}
	}
	return td, nil
}

func (r *redactor) processLogs(_ context.Context, ld plog.Logs) (plog.Logs, error) {
	rls := ld.ResourceLogs()
	for i := 0; i < rls.Len(); i++ {
		rl := rls.At(i)
		r.enrichResource(rl.Resource().Attributes())
		sls := rl.ScopeLogs()
		for j := 0; j < sls.Len(); j++ {
			lrs := sls.At(j).LogRecords()
			for k := 0; k < lrs.Len(); k++ {
				lr := lrs.At(k)
				r.sanitizeAttrs(lr.Attributes())
				// Prompts are frequently logged in the body — string, map, or slice.
				r.sanitizeValue(lr.Body())
			}
		}
	}
	return ld, nil
}

// enrichResource stamps deployment.environment (when configured) on every
// resource, and sanitizes resource attributes — they are attached to every
// span/log and are copied onto metric labels by resource_to_telemetry_conversion.
func (r *redactor) enrichResource(attrs pcommon.Map) {
	if r.cfg.Environment != "" {
		attrs.PutStr(deploymentEnvironmentKey, r.cfg.Environment)
	}
	r.redactKeys(attrs)
	r.scrubStrings(attrs)
}

// sanitizeSpan sanitizes a span's attributes, name, and events. Prompt/response
// content is commonly carried as span events by some instrumentations, so those
// must be redacted too.
func (r *redactor) sanitizeSpan(span ptrace.Span) {
	r.sanitizeAttrs(span.Attributes())
	if out, changed := r.scrubber.Scrub(span.Name()); changed {
		span.SetName(out)
	}
	events := span.Events()
	for i := 0; i < events.Len(); i++ {
		ev := events.At(i)
		r.redactKeys(ev.Attributes())
		r.scrubStrings(ev.Attributes())
		if out, changed := r.scrubber.Scrub(ev.Name()); changed {
			ev.SetName(out)
		}
	}
}

// sanitizeAttrs applies, in order: provider normalization, cost enrichment,
// content redaction, and PII scrubbing.
func (r *redactor) sanitizeAttrs(attrs pcommon.Map) {
	r.normalizeProvider(attrs)
	r.enrichCost(attrs)
	r.redactKeys(attrs)
	r.scrubStrings(attrs)
}

// sanitizeValue redacts sensitive keys and scrubs PII within a value, recursing
// through map and slice bodies (e.g. structured log bodies).
func (r *redactor) sanitizeValue(v pcommon.Value) {
	switch v.Type() {
	case pcommon.ValueTypeStr:
		if out, changed := r.scrubber.Scrub(v.Str()); changed {
			v.SetStr(out)
		}
	case pcommon.ValueTypeMap:
		m := v.Map()
		r.redactKeys(m)
		m.Range(func(_ string, mv pcommon.Value) bool {
			r.sanitizeValue(mv)
			return true
		})
	case pcommon.ValueTypeSlice:
		s := v.Slice()
		for i := 0; i < s.Len(); i++ {
			r.sanitizeValue(s.At(i))
		}
	}
}

func (r *redactor) normalizeProvider(attrs pcommon.Map) {
	if !r.cfg.NormalizeProvider {
		return
	}
	sys, ok := attrs.Get(genai.AttrSystem)
	if !ok {
		return
	}
	if _, exists := attrs.Get(genai.AttrProviderName); !exists {
		attrs.PutStr(genai.AttrProviderName, sys.AsString())
	}
	attrs.Remove(genai.AttrSystem)
}

// enrichCost derives gen_ai.usage.cost_usd from token counts before the token
// attributes could be touched by anything downstream.
func (r *redactor) enrichCost(attrs pcommon.Map) {
	model := genai.StrAttr(attrs, genai.AttrRequestModel)
	if model == "" {
		return
	}
	in := genai.IntAttr(attrs, genai.AttrUsageInputTokens)
	out := genai.IntAttr(attrs, genai.AttrUsageOutputTokens)
	if in == 0 && out == 0 {
		return
	}
	if cost, ok := genai.EstimateCostUSD(r.cfg.ModelPricing, model, in, out); ok {
		attrs.PutDouble(genai.AttrUsageCostUSD, cost)
	}
}

func (r *redactor) redactKeys(attrs pcommon.Map) {
	for key := range r.redact {
		if _, ok := attrs.Get(key); !ok {
			continue
		}
		switch r.cfg.Mode {
		case ModeDelete:
			attrs.Remove(key)
		case ModeHash:
			v, _ := attrs.Get(key)
			sum := sha256.Sum256([]byte(r.cfg.Salt + v.AsString()))
			attrs.PutStr(key, "sha256:"+hex.EncodeToString(sum[:]))
		default: // ModeMask
			attrs.PutStr(key, r.cfg.MaskString)
		}
	}
}

// scrubStrings masks PII in every remaining string attribute value. Values we
// just wrote (mask string, hash, cost) will not match the patterns.
func (r *redactor) scrubStrings(attrs pcommon.Map) {
	attrs.Range(func(_ string, v pcommon.Value) bool {
		if v.Type() != pcommon.ValueTypeStr {
			return true
		}
		if out, changed := r.scrubber.Scrub(v.Str()); changed {
			v.SetStr(out)
		}
		return true
	})
}
