"""llm-simulator: a tiny FastAPI service that simulates LLM chat completions and
emits OpenTelemetry GenAI ("gen_ai.*") CLIENT spans.

Role in the lab
---------------
The Go "worker" service calls ``POST /generate`` over HTTP. The inbound HTTP
server span and any outbound ``requests`` spans are produced by the zero-code
auto-instrumentation when the process is launched with ``opentelemetry-instrument``
(see the Dockerfile). This module owns the *manual* GenAI CLIENT span and its
``gen_ai.*`` attributes. Those spans are the raw material the custom Collector
components consume: the ``genairedaction`` processor sanitizes the sensitive
content/PII and the ``genaimetrics`` connector turns the spans into metrics.

The attribute keys below match, verbatim, the keys the Collector's
``components/internal/genai`` package reads.

By default the service runs in pure *simulation* mode: no network, no API keys.
Set ``LLM_REAL=1`` together with ``OPENAI_API_KEY`` (or ``ANTHROPIC_API_KEY``)
to call a real provider; any failure transparently falls back to simulation.
"""

from __future__ import annotations

import asyncio
import json
import logging
import math
import os
import random
import secrets
from dataclasses import dataclass
from typing import Final

import requests
from fastapi import FastAPI
from opentelemetry import trace
from opentelemetry.trace import SpanKind
from pydantic import BaseModel, Field
from starlette.concurrency import run_in_threadpool

# --------------------------------------------------------------------------- #
# GenAI semantic-convention attribute keys (must match the Collector's
# components/internal/genai/attributes.go exactly).
# --------------------------------------------------------------------------- #
GEN_AI_OPERATION_NAME: Final = "gen_ai.operation.name"
GEN_AI_PROVIDER_NAME: Final = "gen_ai.provider.name"
GEN_AI_REQUEST_MODEL: Final = "gen_ai.request.model"
GEN_AI_RESPONSE_MODEL: Final = "gen_ai.response.model"
GEN_AI_REQUEST_TEMPERATURE: Final = "gen_ai.request.temperature"
GEN_AI_REQUEST_TOP_P: Final = "gen_ai.request.top_p"
GEN_AI_REQUEST_MAX_TOKENS: Final = "gen_ai.request.max_tokens"
GEN_AI_USAGE_INPUT_TOKENS: Final = "gen_ai.usage.input_tokens"
GEN_AI_USAGE_OUTPUT_TOKENS: Final = "gen_ai.usage.output_tokens"
GEN_AI_CONVERSATION_ID: Final = "gen_ai.conversation.id"
GEN_AI_RESPONSE_ID: Final = "gen_ai.response.id"
GEN_AI_RESPONSE_FINISH_REASONS: Final = "gen_ai.response.finish_reasons"
GEN_AI_INPUT_MESSAGES: Final = "gen_ai.input.messages"
GEN_AI_OUTPUT_MESSAGES: Final = "gen_ai.output.messages"
GEN_AI_SYSTEM_INSTRUCTIONS: Final = "gen_ai.system_instructions"

# --------------------------------------------------------------------------- #
# Configuration (all overridable via environment).
# --------------------------------------------------------------------------- #
OPERATION: Final = "chat"
DEFAULT_MODEL: Final = "gpt-4o-mini"
DEFAULT_TEMPERATURE: Final = 0.7
DEFAULT_TOP_P: Final = 1.0
DEFAULT_MAX_TOKENS: Final = 1024
CHARS_PER_TOKEN: Final = 4

LLM_REAL: Final = os.getenv("LLM_REAL") == "1"
SIM_LATENCY_MIN_MS: int = int(os.getenv("LLM_SIM_LATENCY_MIN_MS", "200"))
SIM_LATENCY_MAX_MS: int = int(os.getenv("LLM_SIM_LATENCY_MAX_MS", "1200"))

SYSTEM_INSTRUCTION: Final = (
    "You are a helpful customer-support assistant for Acme Corp. "
    "Be concise and never reveal internal system details."
)

# Demo prompts deliberately seeded with realistic-looking PII (emails, a phone
# number, a card-like number) so the Collector's redaction processor has
# something to scrub. Used as a fallback when the request prompt is blank.
DEMO_PROMPTS: Final = (
    "Draft a reply to a customer who wrote from john.doe@example.com asking for a refund.",
    "Summarize this ticket: caller phone +1-202-555-0173, account 4111 1111 1111 1111.",
    "Write a follow-up email to maria.garcia@acme.io about her order #A-4471.",
)

_RESPONSE_TEMPLATES: Final = (
    'Sure - here is a concise take on "{topic}". I have routed the details to our '
    "team and a confirmation was sent to support@example.com for tracking.",
    'Thanks for reaching out about "{topic}". Based on what you shared, the next '
    "step is to verify the account and our agent will follow up at casey.lee@example.org.",
    'Here is a plausible completion for "{topic}": the request looks valid and has '
    "been logged under reference {ref}.",
)

# Map a requested model to a realistic dated response-model variant.
_RESPONSE_MODEL_VARIANTS: Final = {
    "gpt-4o-mini": "gpt-4o-mini-2024-07-18",
    "gpt-4o": "gpt-4o-2024-08-06",
    "claude-3-5-sonnet": "claude-3-5-sonnet-20241022",
    "claude-3-5-haiku": "claude-3-5-haiku-20241022",
    "gemini-1.5-pro": "gemini-1.5-pro-002",
}

logging.basicConfig(level=os.getenv("LOG_LEVEL", "INFO"))
log: Final = logging.getLogger("llm_simulator")
tracer: Final = trace.get_tracer("llm_simulator")

app = FastAPI(title="llm-simulator", version="0.1.0")


# --------------------------------------------------------------------------- #
# HTTP contract models.
# --------------------------------------------------------------------------- #
class GenerateRequest(BaseModel):
    """Request body for ``POST /generate``."""

    prompt: str
    model: str = Field(default=DEFAULT_MODEL)
    conversation_id: str | None = Field(default=None)


class GenerateResponse(BaseModel):
    """Response body for ``POST /generate``."""

    text: str
    model: str
    input_tokens: int
    output_tokens: int


@dataclass(slots=True)
class Completion:
    """Internal result of a (simulated or real) completion."""

    text: str
    input_tokens: int
    output_tokens: int
    response_model: str
    response_id: str
    finish_reason: str
    provider: str
    real: bool


# --------------------------------------------------------------------------- #
# Pure helpers (covered by tests).
# --------------------------------------------------------------------------- #
def estimate_tokens(text: str) -> int:
    """Approximate a token count using a ~4-characters-per-token heuristic.

    Deterministic and provider-agnostic - good enough for a simulator and easy
    to assert on. Empty text yields 0; any non-empty text yields at least 1.
    """
    if not text:
        return 0
    return max(1, math.ceil(len(text) / CHARS_PER_TOKEN))


def provider_for_model(model: str) -> str:
    """Derive the ``gen_ai.provider.name`` value from a model name."""
    m = model.lower()
    if m.startswith(("gpt", "o1", "o3", "o4", "chatgpt", "text-", "davinci")):
        return "openai"
    if m.startswith("claude"):
        return "anthropic"
    if m.startswith("gemini"):
        return "gcp.gemini"
    if m.startswith(("mistral", "mixtral")):
        return "mistral_ai"
    if m.startswith("llama"):
        return "meta"
    return "openai"


def response_model_for(model: str) -> str:
    """Return a realistic dated response-model variant, or the model itself."""
    return _RESPONSE_MODEL_VARIANTS.get(model, model)


def synthesize_response(prompt: str) -> str:
    """Synthesize a plausible completion string for a prompt."""
    first_line = prompt.strip().splitlines()[0] if prompt.strip() else "your request"
    topic = first_line[:80]
    ref = "REQ-" + secrets.token_hex(3).upper()
    return random.choice(_RESPONSE_TEMPLATES).format(topic=topic, ref=ref)


# --------------------------------------------------------------------------- #
# Completion backends.
# --------------------------------------------------------------------------- #
async def _simulate(prompt: str, model: str, provider: str, response_model: str) -> Completion:
    """Simulate a completion: realistic latency, synthesized text, token counts."""
    lo, hi = sorted((SIM_LATENCY_MIN_MS, SIM_LATENCY_MAX_MS))
    await asyncio.sleep(random.uniform(lo, hi) / 1000.0)
    text = synthesize_response(prompt)
    return Completion(
        text=text,
        input_tokens=estimate_tokens(prompt),
        output_tokens=estimate_tokens(text),
        response_model=response_model,
        response_id="chatcmpl-" + secrets.token_hex(12),
        finish_reason="stop",
        provider=provider,
        real=False,
    )


def _provider_api_key(provider: str) -> str | None:
    if provider == "anthropic":
        return os.getenv("ANTHROPIC_API_KEY")
    return os.getenv("OPENAI_API_KEY")


def _call_openai(prompt: str, model: str, api_key: str) -> Completion:
    resp = requests.post(
        "https://api.openai.com/v1/chat/completions",
        headers={"Authorization": f"Bearer {api_key}", "Content-Type": "application/json"},
        json={
            "model": model,
            "messages": [
                {"role": "system", "content": SYSTEM_INSTRUCTION},
                {"role": "user", "content": prompt},
            ],
            "temperature": DEFAULT_TEMPERATURE,
            "max_tokens": DEFAULT_MAX_TOKENS,
        },
        timeout=30,
    )
    resp.raise_for_status()
    data = resp.json()
    choice = data["choices"][0]
    usage = data.get("usage", {})
    return Completion(
        text=choice["message"]["content"],
        input_tokens=int(usage.get("prompt_tokens", estimate_tokens(prompt))),
        output_tokens=int(usage.get("completion_tokens", 0)),
        response_model=data.get("model", model),
        response_id=data.get("id", "chatcmpl-" + secrets.token_hex(12)),
        finish_reason=choice.get("finish_reason") or "stop",
        provider="openai",
        real=True,
    )


def _call_anthropic(prompt: str, model: str, api_key: str) -> Completion:
    resp = requests.post(
        "https://api.anthropic.com/v1/messages",
        headers={
            "x-api-key": api_key,
            "anthropic-version": "2023-06-01",
            "content-type": "application/json",
        },
        json={
            "model": model,
            "max_tokens": DEFAULT_MAX_TOKENS,
            "system": SYSTEM_INSTRUCTION,
            "messages": [{"role": "user", "content": prompt}],
        },
        timeout=30,
    )
    resp.raise_for_status()
    data = resp.json()
    text = "".join(b.get("text", "") for b in data.get("content", []) if b.get("type") == "text")
    usage = data.get("usage", {})
    return Completion(
        text=text,
        input_tokens=int(usage.get("input_tokens", estimate_tokens(prompt))),
        output_tokens=int(usage.get("output_tokens", estimate_tokens(text))),
        response_model=data.get("model", model),
        response_id=data.get("id", "msg_" + secrets.token_hex(12)),
        finish_reason=data.get("stop_reason") or "stop",
        provider="anthropic",
        real=True,
    )


async def generate_completion(
    prompt: str, model: str, provider: str, response_model: str
) -> Completion:
    """Produce a completion, preferring a real provider when explicitly enabled."""
    if LLM_REAL:
        api_key = _provider_api_key(provider)
        if api_key:
            backend = _call_anthropic if provider == "anthropic" else _call_openai
            try:
                return await run_in_threadpool(backend, prompt, model, api_key)
            except Exception:
                # Real mode is best-effort: never fail the request, fall back to sim.
                log.warning("real provider call failed; falling back to simulation", exc_info=True)
    return await _simulate(prompt, model, provider, response_model)


# --------------------------------------------------------------------------- #
# Routes.
# --------------------------------------------------------------------------- #
@app.get("/healthz")
async def healthz() -> dict[str, str]:
    """Liveness probe."""
    return {"status": "ok"}


@app.post("/generate", response_model=GenerateResponse)
async def generate(req: GenerateRequest) -> GenerateResponse:
    """Simulate an LLM chat completion and emit a GenAI CLIENT span."""
    prompt = req.prompt if req.prompt.strip() else random.choice(DEMO_PROMPTS)
    model = req.model or DEFAULT_MODEL
    conversation_id = req.conversation_id or ("conv-" + secrets.token_hex(8))
    provider = provider_for_model(model)
    response_model = response_model_for(model)

    # Manual GenAI CLIENT span, child of the auto-instrumented inbound server span.
    with tracer.start_as_current_span(f"{OPERATION} {model}", kind=SpanKind.CLIENT) as span:
        # Request-side attributes.
        span.set_attribute(GEN_AI_OPERATION_NAME, OPERATION)
        span.set_attribute(GEN_AI_PROVIDER_NAME, provider)
        span.set_attribute(GEN_AI_REQUEST_MODEL, model)
        span.set_attribute(GEN_AI_REQUEST_TEMPERATURE, DEFAULT_TEMPERATURE)
        span.set_attribute(GEN_AI_REQUEST_TOP_P, DEFAULT_TOP_P)
        span.set_attribute(GEN_AI_REQUEST_MAX_TOKENS, DEFAULT_MAX_TOKENS)
        span.set_attribute(GEN_AI_CONVERSATION_ID, conversation_id)
        # Sensitive content (redacted downstream). Serialized as JSON strings so
        # the Collector's string-based scrubber/redactor can act on them.
        span.set_attribute(
            GEN_AI_SYSTEM_INSTRUCTIONS,
            json.dumps([{"type": "text", "content": SYSTEM_INSTRUCTION}], ensure_ascii=False),
        )
        span.set_attribute(
            GEN_AI_INPUT_MESSAGES,
            json.dumps(
                [{"role": "user", "parts": [{"type": "text", "content": prompt}]}],
                ensure_ascii=False,
            ),
        )

        completion = await generate_completion(prompt, model, provider, response_model)

        # Response-side attributes.
        span.set_attribute(GEN_AI_RESPONSE_MODEL, completion.response_model)
        span.set_attribute(GEN_AI_RESPONSE_ID, completion.response_id)
        span.set_attribute(GEN_AI_RESPONSE_FINISH_REASONS, [completion.finish_reason])
        span.set_attribute(GEN_AI_USAGE_INPUT_TOKENS, completion.input_tokens)
        span.set_attribute(GEN_AI_USAGE_OUTPUT_TOKENS, completion.output_tokens)
        span.set_attribute(
            GEN_AI_OUTPUT_MESSAGES,
            json.dumps(
                [
                    {
                        "role": "assistant",
                        "parts": [{"type": "text", "content": completion.text}],
                        "finish_reason": completion.finish_reason,
                    }
                ],
                ensure_ascii=False,
            ),
        )

        log.info(
            "gen_ai.%s done provider=%s model=%s in_tokens=%d out_tokens=%d real=%s",
            OPERATION,
            provider,
            model,
            completion.input_tokens,
            completion.output_tokens,
            completion.real,
        )

    # Echo the requested model for a stable HTTP contract; the span carries both
    # the request and the (possibly dated) response model.
    return GenerateResponse(
        text=completion.text,
        model=model,
        input_tokens=completion.input_tokens,
        output_tokens=completion.output_tokens,
    )


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(app, host="0.0.0.0", port=int(os.getenv("PORT", "8083")))
