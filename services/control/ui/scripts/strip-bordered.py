import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1] / "src"

PATTERNS = [
    re.compile(r'\s:bordered="true"'),
    re.compile(r'\s+bordered(?=\s|>|/)'),
]


def strip_bordered(text: str) -> str:
    for pat in PATTERNS:
        text = pat.sub("", text)
    return text


def main() -> None:
    changed: list[str] = []
    for path in sorted(ROOT.rglob("*.vue")):
        text = path.read_text(encoding="utf-8")
        updated = strip_bordered(text)
        if updated != text:
            path.write_text(updated, encoding="utf-8", newline="\n")
            changed.append(str(path.relative_to(ROOT)))
    print(f"updated {len(changed)} files")
    for name in changed:
        print(f"  {name}")


if __name__ == "__main__":
    main()
