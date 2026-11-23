from typing import Any

import pytest


@pytest.fixture
def finished_request() -> dict[str, Any]:
    return {
        "id": "7d1a0f4e-1111-4222-8333-444455556666",
        "state": "succeeded",
        "failure_code": None,
        "error": None,
        "as_of": "2026-11-20T10:00:00Z",
        "created_at": "2026-11-20T10:05:00Z",
        "finished_at": "2026-11-20T10:05:30Z",
        "steps": [
            {
                "at": "2026-11-20T10:05:05Z",
                "attempt": 1,
                "phase": "discover",
                "tool": "list_labels",
                "status": "ok",
                "message": "Listed 12 labels on prometheus",
                "duration_ms": 40,
            },
            {
                "at": "2026-11-20T10:05:20Z",
                "attempt": 1,
                "phase": "query",
                "tool": "query_range",
                "status": "error",
                "message": "The query was refused",
                "duration_ms": 12,
            },
        ],
        "usage": {
            "input_tokens": 1000,
            "output_tokens": 200,
            "cache_read_tokens": 300,
            "cache_write_tokens": 50,
            "model_calls": 3,
            "tool_calls": 2,
            "cost_usd": 0.0123,
        },
        "finding": {"markdown": "The 5xx share is 2.1%.\nTrace 0123456789abcdef0123456789abcdef is one example."},
    }
