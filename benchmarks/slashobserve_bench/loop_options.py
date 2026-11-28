from typing import Any, Literal

from pydantic import BaseModel, Field, model_validator

SERVER_DEFAULT = "server default"
OLDER_IMAGE_CALLS = 12
DEFAULT_INVESTIGATION_LOOP = True
DEFAULT_INVESTIGATION_CALLS_MAX = 40
DEFAULT_FINDING_GROUNDING = "enforce"
DEFAULT_INVESTIGATION_BRIEF = "on"


class LoopOptions(BaseModel):
    investigation_calls: int | None = Field(default=None, ge=6, le=60)
    investigation_loop: bool | None = None
    investigation_calls_max: int | None = Field(default=None, ge=6, le=60)
    finding_grounding: Literal["off", "enforce"] | None = None
    investigation_brief: Literal["on", "off"] | None = None

    @property
    def effective_loop(self) -> bool:
        return DEFAULT_INVESTIGATION_LOOP if self.investigation_loop is None else self.investigation_loop

    @property
    def effective_calls_max(self) -> int:
        if self.investigation_calls_max is not None:
            return self.investigation_calls_max
        return max(DEFAULT_INVESTIGATION_CALLS_MAX, self.investigation_calls or 0)

    @property
    def effective_grounding(self) -> str:
        return self.finding_grounding or DEFAULT_FINDING_GROUNDING

    @property
    def effective_brief(self) -> str:
        return self.investigation_brief or DEFAULT_INVESTIGATION_BRIEF

    def grounding_note(self) -> str:
        source = "as given" if self.finding_grounding else "the adapter's default"
        return f"finding grounding: {self.effective_grounding} ({source}); causes are scored and banded only with report or enforce, and an image without the setting ignores it"

    def loop_note(self) -> str:
        calls = self.investigation_calls or f"{SERVER_DEFAULT}, by task type on an image with task types and {OLDER_IMAGE_CALLS} on an older one"
        return (
            f"investigation loop: {str(self.effective_loop).lower()}, calls max {self.effective_calls_max}, calls {calls}, "
            f"preflight sweep: {self.effective_brief}; the loop, calls max and sweep are always sent, and an image without a setting ignores it"
        )

    @model_validator(mode="after")
    def _max_covers_calls(self) -> "LoopOptions":
        floor = self.investigation_calls or OLDER_IMAGE_CALLS
        if self.investigation_calls_max is not None and self.investigation_calls_max < floor:
            raise ValueError(f"investigation_calls_max must not be below the investigation calls ({floor})")
        return self

    def server_settings(self) -> dict[str, Any]:
        return {
            "investigation_calls": self.investigation_calls,
            "investigation_loop": self.effective_loop,
            "investigation_calls_max": self.effective_calls_max,
            "finding_grounding": self.effective_grounding,
            "investigation_brief": self.effective_brief,
        }

    def recorded_loop_settings(self) -> dict[str, Any]:
        return {
            **self.server_settings(),
            "investigation_calls": self.investigation_calls or SERVER_DEFAULT,
        }
