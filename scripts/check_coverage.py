"""Fail when any service.py is under the per-file minimum. Reads coverage.json."""

import json
import sys
from pathlib import Path

MINIMUM = 90.0


def main() -> int:
    data = json.loads(Path(sys.argv[1] if len(sys.argv) > 1 else "coverage.json").read_text())
    low = {
        name: info["summary"]["percent_covered"]
        for name, info in data["files"].items()
        if name.endswith("service.py") and info["summary"]["percent_covered"] < MINIMUM
    }
    for name, pct in sorted(low.items()):
        print(f"{name}: {pct:.1f}% < {MINIMUM:.0f}%")
    return 1 if low else 0


if __name__ == "__main__":
    sys.exit(main())
