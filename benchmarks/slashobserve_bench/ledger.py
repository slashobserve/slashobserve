import fcntl
import json
from contextlib import contextmanager
from datetime import UTC, datetime
from pathlib import Path


class SpendCapReached(Exception):
    def __init__(self, spent_usd: float, cap_usd: float):
        super().__init__(f"the job spend cap of ${cap_usd:.2f} is reached (${spent_usd:.4f} spent)")
        self.spent_usd = spent_usd
        self.cap_usd = cap_usd


class SpendLedger:
    def __init__(self, path: Path, cap_usd: float):
        self.path = path
        self.cap_usd = cap_usd

    @contextmanager
    def _locked(self):
        self.path.parent.mkdir(parents=True, exist_ok=True)
        with self.path.open("a+", encoding="utf-8") as handle:
            fcntl.flock(handle, fcntl.LOCK_EX)
            try:
                handle.seek(0)
                yield handle
            finally:
                fcntl.flock(handle, fcntl.LOCK_UN)

    @staticmethod
    def _total(handle) -> float:
        return sum(json.loads(line)["cost_usd"] for line in handle if line.strip())

    def total(self) -> float:
        with self._locked() as handle:
            return self._total(handle)

    def ensure_room(self) -> None:
        spent = self.total()
        if spent >= self.cap_usd:
            raise SpendCapReached(spent, self.cap_usd)

    def record(self, trial: str, cost_usd: float) -> float:
        entry = {"trial": trial, "cost_usd": cost_usd, "at": datetime.now(UTC).isoformat()}
        with self._locked() as handle:
            handle.seek(0, 2)
            handle.write(json.dumps(entry) + "\n")
            handle.flush()
            handle.seek(0)
            return self._total(handle)
