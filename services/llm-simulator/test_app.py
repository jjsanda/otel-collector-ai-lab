"""Tests for the llm-simulator: token estimation and the /generate contract.

These run without ``opentelemetry-instrument``; the manual span code executes
against a no-op tracer provider, which is exactly the resilience we want.
"""

from __future__ import annotations

import pytest
from fastapi.testclient import TestClient

import app as appmod
from app import GenerateResponse, estimate_tokens, provider_for_model, response_model_for

client = TestClient(appmod.app)


@pytest.fixture(autouse=True)
def _fast_sim(monkeypatch: pytest.MonkeyPatch) -> None:
    """Keep simulated latency negligible so the suite stays fast."""
    monkeypatch.setattr(appmod, "SIM_LATENCY_MIN_MS", 0)
    monkeypatch.setattr(appmod, "SIM_LATENCY_MAX_MS", 1)


# --------------------------------------------------------------------------- #
# Token estimation.
# --------------------------------------------------------------------------- #
@pytest.mark.parametrize(
    ("text", "expected"),
    [
        ("", 0),
        ("a", 1),
        ("abcd", 1),
        ("abcde", 2),
        ("a" * 40, 10),
    ],
)
def test_estimate_tokens(text: str, expected: int) -> None:
    assert estimate_tokens(text) == expected


def test_estimate_tokens_is_monotonic() -> None:
    assert estimate_tokens("a" * 100) > estimate_tokens("a" * 10)


def test_provider_for_model() -> None:
    assert provider_for_model("gpt-4o-mini") == "openai"
    assert provider_for_model("claude-3-5-sonnet") == "anthropic"
    assert provider_for_model("gemini-1.5-pro") == "gcp.gemini"
    assert provider_for_model("something-unknown") == "openai"


def test_response_model_variant() -> None:
    assert response_model_for("gpt-4o-mini") == "gpt-4o-mini-2024-07-18"
    assert response_model_for("custom-model") == "custom-model"


# --------------------------------------------------------------------------- #
# /healthz.
# --------------------------------------------------------------------------- #
def test_healthz() -> None:
    resp = client.get("/healthz")
    assert resp.status_code == 200
    assert resp.json() == {"status": "ok"}


# --------------------------------------------------------------------------- #
# /generate.
# --------------------------------------------------------------------------- #
def test_generate_response_shape() -> None:
    resp = client.post(
        "/generate",
        json={"prompt": "hello, my email is a@b.com", "model": "gpt-4o-mini"},
    )
    assert resp.status_code == 200
    body = resp.json()

    # Exact contract shape, no extra keys.
    assert set(body) == {"text", "model", "input_tokens", "output_tokens"}
    # Validates types against the declared response model too.
    parsed = GenerateResponse(**body)
    assert parsed.text
    assert parsed.model == "gpt-4o-mini"
    assert parsed.input_tokens > 0
    assert parsed.output_tokens > 0


def test_generate_defaults_model_when_omitted() -> None:
    resp = client.post("/generate", json={"prompt": "summarize the latest news"})
    assert resp.status_code == 200
    assert resp.json()["model"] == "gpt-4o-mini"


def test_generate_input_tokens_track_prompt_length() -> None:
    prompt = "x" * 80
    resp = client.post("/generate", json={"prompt": prompt})
    assert resp.status_code == 200
    # Simulation mode (the default) derives input tokens straight from the prompt.
    assert resp.json()["input_tokens"] == estimate_tokens(prompt)


def test_generate_blank_prompt_uses_demo_prompt() -> None:
    # A blank prompt still produces a valid completion (a PII-bearing demo prompt).
    resp = client.post("/generate", json={"prompt": "   "})
    assert resp.status_code == 200
    assert resp.json()["input_tokens"] > 0


def test_generate_requires_prompt() -> None:
    resp = client.post("/generate", json={"model": "gpt-4o-mini"})
    assert resp.status_code == 422
