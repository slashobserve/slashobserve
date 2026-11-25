#!/usr/bin/env bash
set -euo pipefail

orca_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
module="$(uv run --locked --project "$orca_dir" python -c 'import harbor.models.task.task as m; print(m.__file__)')"
if grep -q 'ignore=\[".trials/"\]' "$module"; then
  echo "already patched: $module"
  exit 0
fi
grep -q 'dirhash(self._task_dir, "sha256")' "$module" || { echo "unexpected Harbor source in $module" >&2; exit 1; }
python3 - "$module" <<'PY'
import sys
path = sys.argv[1]
text = open(path, encoding="utf-8").read()
open(path, "w", encoding="utf-8").write(text.replace('dirhash(self._task_dir, "sha256")', 'dirhash(self._task_dir, "sha256", ignore=[".trials/"])'))
PY
echo "patched: $module"
