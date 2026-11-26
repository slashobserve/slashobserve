from typing import Any, Literal

from pydantic import BaseModel, Field, model_validator

DEFAULT_INVESTIGATION_CALLS = 12


class LoopOptions(BaseModel):
    investigation_calls: int | None = Field(default=None, ge=6, le=60)
    investigation_loop: bool | None = None
    investigation_calls_max: int | None = Field(default=None, ge=6, le=60)
    finding_grounding: Literal["off", "report", "enforce"] | None = None

    @property
    def effective_calls(self) -> int:
        return self.investigation_calls or DEFAULT_INVESTIGATION_CALLS

    @model_validator(mode="after")
    def _max_covers_calls(self) -> "LoopOptions":
        if self.investigation_calls_max is not None and self.investigation_calls_max < self.effective_calls:
            raise ValueError(f"investigation_calls_max must not be below the investigation calls ({self.effective_calls})")
        return self

    def effective_loop_settings(self) -> dict[str, Any]:
        return {
            "investigation_loop": bool(self.investigation_loop),
            "investigation_calls_max": self.investigation_calls_max or self.effective_calls,
            "finding_grounding": self.finding_grounding or "off",
        }
