#!/usr/bin/env python3

import argparse
import pathlib
import re
import sys


REQUIRED_TARGETS = (
    ("darwin", "amd64"),
    ("darwin", "arm64"),
    ("linux", "amd64"),
    ("linux", "arm64"),
)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Generate skillctl Homebrew formula")
    parser.add_argument("--dist-dir", required=True)
    parser.add_argument("--checksums-file", required=True)
    parser.add_argument("--tag", required=True)
    parser.add_argument("--repo", required=True)
    parser.add_argument("--output", required=True)
    return parser.parse_args()


def load_checksums(path: pathlib.Path) -> dict[str, str]:
    checksums: dict[str, str] = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line:
            continue

        parts = line.split(None, 1)
        if len(parts) != 2:
            raise ValueError(f"invalid checksum line: {line}")

        sha, filename = parts
        filename = filename.strip()
        if filename.startswith("*"):
            filename = filename[1:]
        checksums[filename] = sha

    return checksums


def discover_archives(dist_dir: pathlib.Path) -> dict[tuple[str, str], str]:
    archives: dict[tuple[str, str], str] = {}
    pattern = re.compile(r"^.+_(darwin|linux)_(amd64|arm64)(?:_.+)?\.tar\.gz$")

    for path in sorted(dist_dir.glob("*.tar.gz")):
        match = pattern.match(path.name)
        if not match:
            continue

        key = (match.group(1), match.group(2))
        archives[key] = path.name

    return archives


def require_targets(archives: dict[tuple[str, str], str]) -> None:
    missing = [
        f"{os_name}/{arch}"
        for os_name, arch in REQUIRED_TARGETS
        if (os_name, arch) not in archives
    ]
    if missing:
        raise ValueError(f"missing required archives in dist: {', '.join(missing)}")


def build_formula(
    tag: str, repo: str, assets: dict[tuple[str, str], tuple[str, str]]
) -> str:
    version = tag[1:] if tag.startswith("v") else tag

    darwin_amd64_filename, darwin_amd64_sha = assets[("darwin", "amd64")]
    darwin_arm64_filename, darwin_arm64_sha = assets[("darwin", "arm64")]
    linux_amd64_filename, linux_amd64_sha = assets[("linux", "amd64")]
    linux_arm64_filename, linux_arm64_sha = assets[("linux", "arm64")]

    return f'''class Skillctl < Formula
  desc "Interactive TUI for syncing AI agent skills"
  homepage "https://github.com/{repo}"
  version "{version}"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/{repo}/releases/download/{tag}/{darwin_arm64_filename}"
      sha256 "{darwin_arm64_sha}"
    else
      url "https://github.com/{repo}/releases/download/{tag}/{darwin_amd64_filename}"
      sha256 "{darwin_amd64_sha}"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/{repo}/releases/download/{tag}/{linux_arm64_filename}"
      sha256 "{linux_arm64_sha}"
    else
      url "https://github.com/{repo}/releases/download/{tag}/{linux_amd64_filename}"
      sha256 "{linux_amd64_sha}"
    end
  end

  def install
    bin.install "skillctl"
  end

  test do
    assert_match "skillctl", shell_output("#{{bin}}/skillctl --help")
  end
end
'''


def main() -> int:
    args = parse_args()

    dist_dir = pathlib.Path(args.dist_dir)
    checksums_file = pathlib.Path(args.checksums_file)
    output_file = pathlib.Path(args.output)

    checksums = load_checksums(checksums_file)
    archives = discover_archives(dist_dir)
    require_targets(archives)

    assets: dict[tuple[str, str], tuple[str, str]] = {}
    for key, filename in archives.items():
        sha = checksums.get(filename)
        if not sha:
            raise ValueError(f"missing checksum for archive: {filename}")
        assets[key] = (filename, sha)

    formula = build_formula(args.tag, args.repo, assets)
    output_file.parent.mkdir(parents=True, exist_ok=True)
    output_file.write_text(formula, encoding="utf-8")

    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f"error: {exc}", file=sys.stderr)
        raise SystemExit(1)
