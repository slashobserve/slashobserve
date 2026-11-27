from typing import Any, Literal

from pydantic import BaseModel, Field, model_validator

DEFAULT_INVESTIGATION_CALLS = 12
DEFAULT_FINDING_GROUNDING = "enforce"


class LoopOptions(BaseModel):
    investigation_calls: int | None = Field(default=None, ge=6, le=60)
    investigation_loop: bool | None = None
    investigation_calls_max: int | None = Field(default=None, ge=6, le=60)
    finding_grounding: Literal["off", "report", "enforce"] | None = None

    @property
    def effective_calls(self) -> int:
        return self.investigation_calls or DEFAULT_INVESTIGATION_CALLS

    @property
    def effective_grounding(self) -> str:
        return self.finding_grounding or DEFAULT_FINDING_GROUNDING

    def grounding_note(self) -> str:
        source = "as given" if self.finding_grounding else "the adapter's default"
        return f"finding grounding: {self.effective_grounding} ({source}); causes are scored and banded only with report or enforce, and an image without the setting ignores it"

    @model_validator(mode="after")
    def _max_covers_calls(self) -> "LoopOptions":
        if self.investigation_calls_max is not None and self.investigation_calls_max < self.effective_calls:
            raise ValueError(f"investigation_calls_max must not be below the investigation calls ({self.effective_calls})")
        return self

    def effective_loop_settings(self) -> dict[str, Any]:
        return {
            "investigation_loop": bool(self.investigation_loop),
            "investigation_calls_max": self.investigation_calls_max or self.effective_calls,
            "finding_grounding": self.effective_grounding,
        }
