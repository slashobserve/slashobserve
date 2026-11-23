import argparse
import json
import sys
from pathlib import Path

TEMPLATE = Path(__file__).resolve().parent.parent / "fixtures" / "store-answer.json"


def fixture_for(instruction: str) -> dict:
    fixture = json.loads(TEMPLATE.read_text(encoding="utf-8"))
    fixture["prompt"] = instruction.strip()
    return fixture


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
