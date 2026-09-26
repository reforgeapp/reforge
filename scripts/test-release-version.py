#!/usr/bin/env python3
import os
from pathlib import Path
import subprocess
import tempfile


script = Path(__file__).with_name("release-version.sh").resolve()
env = dict(os.environ, GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL="/dev/null", GIT_AUTHOR_NAME="Release fixture", GIT_AUTHOR_EMAIL="release@example.test",
           GIT_COMMITTER_NAME="Release fixture", GIT_COMMITTER_EMAIL="release@example.test",
           GIT_AUTHOR_DATE="2026-01-01T00:00:00Z", GIT_COMMITTER_DATE="2026-01-01T00:00:00Z")

with tempfile.TemporaryDirectory(prefix="reforge-release-version-") as directory:
    def git(*args):
        return subprocess.check_output(["git", *args], cwd=directory, env=env, text=True).strip()

    def expect(want, *args):
        result = subprocess.run([str(script), *args], cwd=directory, env=env,
                                text=True, capture_output=True)
        if want is None:
            if result.returncode == 0 or result.stdout or not result.stderr:
                raise AssertionError(f"Expected rejection for {args}: {result}")
        elif result.returncode != 0 or result.stdout.strip() != want:
            raise AssertionError(f"Expected {want} for {args}: {result}")

    git("init", "--quiet", "--initial-branch=master")
    git("-c", "commit.gpgsign=false", "commit", "--quiet", "--allow-empty", "-m", "baseline")
    baseline = git("rev-parse", "HEAD")
    expect(None, "1")
    for invalid in ("", "0", "01", "-1", "1.5", "abc"):
        expect(None, invalid)
    expect(None)
    expect(None, "1", "HEAD", "unexpected")
    expect(None, "1", "missing-commit")
    expect(None, "1", "")
    expect(None, "1", "--help")

    git("tag", "-a", "v0.1", "-m", "Initial release line")
    expect("0.1.1", "1")
    expect("0.1.2", "2")
    expect("0.1.2", "2", "v0.1")
    for ignored in ("v8.9.10", "v9.0-rc.1", "v99.01", "v099.1"):
        git("tag", ignored)
    expect("0.1.3", "3")

    git("-c", "commit.gpgsign=false", "commit", "--quiet", "--allow-empty", "-m", "next line")
    git("tag", "v0.9")
    git("tag", "v0.10")
    expect("0.10.4", "4")
    expect("0.1.4", "4", baseline)
    git("tag", "v1.0")
    expect("1.0.5", "5")
    expect("1.0.5", "5")

    git("switch", "--quiet", "-c", "unreleased")
    git("-c", "commit.gpgsign=false", "commit", "--quiet", "--allow-empty", "-m", "unreleased line")
    git("tag", "v9.9")
    expect("9.9.6", "6")
    git("switch", "--quiet", "master")
    expect("1.0.6", "6")

print("Release version fixtures passed")
