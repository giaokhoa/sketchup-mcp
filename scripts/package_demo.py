#!/usr/bin/env python3
"""Build the complete local Windows SketchUp MCP demo bundle."""

from __future__ import annotations

import argparse
import hashlib
import os
from pathlib import Path
import subprocess

from package_rbz import ARCHIVE_NAME, build as build_rbz

EXE_NAME = "sketchup-mcp-windows-amd64.exe"
SUMS_NAME = "SHA256SUMS.txt"


def run(command: list[str], *, cwd: Path, env: dict[str, str] | None = None) -> None:
    print("+", " ".join(command))
    subprocess.run(command, cwd=cwd, env=env, check=True)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def build_exe(repo_root: Path, output: Path) -> None:
    env = os.environ.copy()
    env.update(
        {
            "CGO_ENABLED": "0",
            "GOOS": "windows",
            "GOARCH": "amd64",
        }
    )
    run(
        [
            "go",
            "build",
            "-trimpath",
            "-buildvcs=false",
            "-o",
            str(output),
            "./cmd/sketchup-mcp",
        ],
        cwd=repo_root,
        env=env,
    )


def write_checksums(output_dir: Path) -> None:
    names = [EXE_NAME, ARCHIVE_NAME]
    lines = [f"{sha256(output_dir / name)}  {name}\n" for name in names]
    (output_dir / SUMS_NAME).write_text("".join(lines), encoding="ascii", newline="\n")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output-dir", type=Path)
    parser.add_argument("--skip-tests", action="store_true")
    args = parser.parse_args()

    repo_root = Path(__file__).resolve().parents[1]
    output_dir = (args.output_dir or repo_root / "dist").resolve()
    output_dir.mkdir(parents=True, exist_ok=True)

    if not args.skip_tests:
        run(["go", "test", "./..."], cwd=repo_root)

    exe = output_dir / EXE_NAME
    rbz = output_dir / ARCHIVE_NAME

    build_exe(repo_root, exe)
    build_rbz(repo_root, rbz)
    write_checksums(output_dir)

    for name in (EXE_NAME, ARCHIVE_NAME, SUMS_NAME):
        print(output_dir / name)


if __name__ == "__main__":
    main()
