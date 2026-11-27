import argparse
import json
import shutil
import sys
from pathlib import Path

TEMPLATE = Path(__file__).resolve().parent.parent / "fixtures" / "store-answer.json"
SENT_SUFFIX = "-sent"


def fixture_for(instruction: str) -> dict:
    fixture = json.loads(TEMPLATE.read_text(encoding="utf-8"))
    fixture["prompt"] = instruction.strip()
    return fixture


def copy_fixtures(source: Path, target: Path) -> None:
    target.mkdir(parents=True, exist_ok=True)
    target.chmod(0o755)
    for path in sorted(source.glob("*.json")):
        copied = target / path.name
        shutil.copyfile(path, copied)
        copied.chmod(0o644)


def rekey(fixture_dir: Path, task_id: str, instruction: str, prompt: str) -> Path | None:
    source = fixture_dir / f"{task_id}.json"
    if prompt.strip() == instruction.strip() or not source.is_file():
        return None
    fixture = json.loads(source.read_text(encoding="utf-8"))
    if fixture.get("prompt", "").strip() != instruction.strip():
        return None
    fixture["prompt"] = prompt.strip()
    target = fixture_dir / f"{task_id}{SENT_SUFFIX}.json"
    target.write_text(json.dumps(fixture, indent=2) + "\n", encoding="utf-8")
    target.chmod(0o644)
    return target


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="python -m slashobserve_bench.replay")
    parser.add_argument("--tasks", type=Path, required=True)
    parser.add_argument("--task", required=True)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args(argv)
    instruction = (args.tasks / args.task / "instruction.md").read_text(encoding="utf-8")
    args.out.mkdir(parents=True, exist_ok=True)
    (args.out / f"{args.task}.json").write_text(json.dumps(fixture_for(instruction), indent=2) + "\n", encoding="utf-8")
    return 0


if __name__ == "__main__":
    sys.exit(main())
