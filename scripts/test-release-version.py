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

    def commit(message):
        git("-c", "commit.gpgsign=false", "commit", "--quiet", "--allow-empty", "-m", message)
        return git("rev-parse", "HEAD")

    git("init", "--quiet", "--initial-branch=master")
    baseline = commit("baseline")
    expect(None)
    for invalid in (("HEAD", "unexpected"), ("missing-commit",), ("",), ("--help",)):
        expect(None, *invalid)

    git("tag", "-a", "v0.1", "-m", "Initial release line")
    expect("0.1.0")
    git("tag", "v0.1.0")
    expect("0.1.0")
    commit("second")
    expect("0.1.1")
    for ignored in ("v8.9.10", "v9.0-rc.1", "v99.01", "v099.1", "v0.1.07"):
        git("tag", ignored)
    expect("0.1.1")
    git("tag", "v0.1.104")
    expect("0.1.104")
    commit("after 104")
    expect("0.1.105")
    expect("0.1.0", baseline)

    git("tag", "v0.2")
    expect("0.2.0")
    git("tag", "v0.2.0")
    commit("third")
    expect("0.2.1")
    git("tag", "v0.10")
    expect("0.10.0")

    git("switch", "--quiet", "-c", "unreleased")
    commit("unreleased line")
    git("tag", "v9.9")
    expect("9.9.0")
    git("switch", "--quiet", "master")
    expect("0.10.0")

print("Release version fixtures passed")
