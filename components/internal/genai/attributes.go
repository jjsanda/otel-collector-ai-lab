// Package genai centralizes the OpenTelemetry GenAI ("gen_ai.*") semantic
// convention attribute keys used across the custom Collector components.
//
// The GenAI semantic conventions are still marked "Development" upstream and
// were recently split into a dedicated repository
// (github.com/open-telemetry/semantic-conventions-genai). Rather than depend on
// a fast-moving semconv module, we pin the exact string keys we target here.
// This mirrors how upstream contrib components handle experimental conventions
// and insulates the build from upstream churn. See docs/adr/0004 for the
// snapshot we target.
package genai

// Core operation/identity attributes.
const (
	// AttrOperationName is the GenAI operation, e.g. "chat", "embeddings",
	// "generate_content", "execute_tool". Used as the first token of a span name.
	AttrOperationName = "gen_ai.operation.name"

	// AttrProviderName identifies the provider, e.g. "openai", "anthropic".
	// This replaces the deprecated "gen_ai.system" attribute.
	AttrProviderName = "gen_ai.provider.name"

	// AttrSystem is the deprecated provider attribute, kept so the redaction
	// processor can normalize it onto AttrProviderName as a showcased enrichment.
	AttrSystem = "gen_ai.system"

	// AttrRequestModel is the model requested, e.g. "gpt-4o-mini".
	AttrRequestModel = "gen_ai.request.model"

	// AttrResponseModel is the model that actually served the response.
	AttrResponseModel = "gen_ai.response.model"

	// AttrConversationID groups spans belonging to one conversation/session.
	AttrConversationID = "gen_ai.conversation.id"

	// AttrResponseID is the provider-assigned response identifier.
	AttrResponseID = "gen_ai.response.id"

	// AttrResponseFinishReasons is a string[] of finish reasons (e.g. "stop").
	AttrResponseFinishReasons = "gen_ai.response.finish_reasons"
)

// Token usage and request-parameter attributes.
const (
	AttrUsageInputTokens  = "gen_ai.usage.input_tokens"
	AttrUsageOutputTokens = "gen_ai.usage.output_tokens"

	// AttrTokenType discriminates a usage measurement: "input" or "output".
	AttrTokenType = "gen_ai.token.type"

	AttrRequestTemperature = "gen_ai.request.temperature"
	AttrRequestTopP        = "gen_ai.request.top_p"
	AttrRequestMaxTokens   = "gen_ai.request.max_tokens"
)

// Sensitive content attributes. These are likely to contain user prompts,
// model responses, and PII; the redaction processor removes/hashes them by
// default.
const (
	AttrInputMessages      = "gen_ai.input.messages"
	AttrOutputMessages     = "gen_ai.output.messages"
	AttrSystemInstructions = "gen_ai.system_instructions"
	AttrToolCallArguments  = "gen_ai.tool.call.arguments"
	AttrToolCallResult     = "gen_ai.tool.call.result"
)

// Enrichment attributes added by this distribution (not part of upstream
// semconv, namespaced under gen_ai.usage / deployment to read naturally).
const (
	// AttrUsageCostUSD is an estimated request cost in USD derived from token
	// counts and a configured price table.
	AttrUsageCostUSD = "gen_ai.usage.cost_usd"
)

// TokenType values.
const (
	TokenTypeInput  = "input"
	TokenTypeOutput = "output"
)

// SensitiveContentKeys returns the attribute keys that carry prompt/response
// content and must be redacted by default.
func SensitiveContentKeys() []string {
	return []string{
		AttrInputMessages,
		AttrOutputMessages,
		AttrSystemInstructions,
		AttrToolCallArguments,
		AttrToolCallResult,
	}
}
