# ADR 0004 — GenAI redaction strategy and semconv snapshot

## Status
Accepted

## Context
GenAI spans carry prompts and responses that routinely contain PII. The
OpenTelemetry GenAI semantic conventions are still in **Development** status and
were recently moved to a dedicated repository
(`open-telemetry/semantic-conventions-genai`), so the attribute set is a moving
target.

## Decision
1. **Redact by default.** The `genairedaction` processor treats
   `gen_ai.input.messages`, `gen_ai.output.messages`, `gen_ai.system_instructions`,
   and `gen_ai.tool.call.arguments`/`result` as sensitive and masks them (with
   `hash` and `delete` modes available). A regex PII scrubber runs over every
   remaining string value as defense-in-depth.
2. **Pin the semconv keys in code.** Rather than depend on a churning semconv Go
   module, the exact `gen_ai.*` key strings live in
   `components/internal/genai/attributes.go`. This mirrors how contrib components
   handle experimental conventions.
3. **Emit semconv-aligned metric names** from the connector
   (`gen_ai.client.token.usage`, `gen_ai.client.operation.duration`) and
   normalize the deprecated `gen_ai.system` → `gen_ai.provider.name`.

## Consequences
- **Pro:** insulated from upstream churn; a single file to update when the
  conventions change.
- **Pro:** redaction is on by default — the safe posture for sensitive data.
- **Con:** the regex scrubber is best-effort, not a substitute for a real DLP
  system; it can miss novel PII shapes or over-match. It is a demonstrator of the
  pattern, and the mask/hash/delete modes are configurable per deployment.
