#!/usr/bin/env python3
"""Build a byte-reproducible SketchUp RBZ from the extension payload only."""

from __future__ import annotations

import argparse
from pathlib import Path
import zipfile

ARCHIVE_NAME = "giaokhoa_sketchup_mcp.rbz"
LOADER_NAME = "giaokhoa_sketchup_mcp.rb"
SUPPORT_NAME = "giaokhoa_sketchup_mcp"
FIXED_TIMESTAMP = (1980, 1, 1, 0, 0, 0)


def archive_info(name: str, *, directory: bool = False) -> zipfile.ZipInfo:
    info = zipfile.ZipInfo(name, FIXED_TIMESTAMP)
    info.create_system = 3
    info.compress_type = zipfile.ZIP_STORED
    info.external_attr = ((0o40755 if directory else 0o100644) << 16)
    if directory:
        info.external_attr |= 0x10
    return info


def build(repo_root: Path, output: Path) -> None:
    sketchup_root = repo_root / "sketchup"
    loader = sketchup_root / LOADER_NAME
    support = sketchup_root / SUPPORT_NAME
    if not loader.is_file() or not support.is_dir():
        raise SystemExit("expected SketchUp loader/support payload is missing")

    files = [loader] + sorted(path for path in support.rglob("*") if path.is_file())
    output.parent.mkdir(parents=True, exist_ok=True)

    with zipfile.ZipFile(output, "w", compression=zipfile.ZIP_STORED) as archive:
        archive.writestr(archive_info(f"{SUPPORT_NAME}/", directory=True), b"")
        archive.writestr(archive_info(LOADER_NAME), loader.read_bytes())
        for path in files[1:]:
            relative = path.relative_to(sketchup_root).as_posix()
            archive.writestr(archive_info(relative), path.read_bytes())


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()

    repo_root = Path(__file__).resolve().parents[1]
    output = args.output or repo_root / "dist" / ARCHIVE_NAME
    build(repo_root, output.resolve())
    print(output.resolve())


if __name__ == "__main__":
    main()
