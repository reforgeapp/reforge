#!/usr/bin/env python3
import argparse
import hashlib
import json
import os
import shutil
import stat
import subprocess
import sys
from pathlib import Path

def fail(message):
    raise SystemExit("prepare-runner: " + message)

def absolute(value, name):
    path = Path(value)
    if not path.is_absolute() or path != Path(os.path.abspath(path)):
        fail(name + " must be absolute and canonical")
    return path

def no_symlink_components(path, name):
    current = Path(path.anchor)
    for part in path.parts[1:]:
        current /= part
        if current.exists() or current.is_symlink():
            if current.is_symlink():
                fail(name + " contains a symlink: " + str(current))

def private_regular(path, name):
    no_symlink_components(path, name)
    try:
        info = path.stat()
    except OSError:
        fail(name + " is unavailable")
    if not stat.S_ISREG(info.st_mode) or info.st_mode & 0o022 or not info.st_mode & 0o111:
        fail(name + " must be a private executable regular file")

def private_directory(path, name):
    no_symlink_components(path, name)
    try:
        info = path.stat()
    except OSError:
        fail(name + " is unavailable")
    if not stat.S_ISDIR(info.st_mode) or info.st_mode & 0o022:
        fail(name + " must be a private directory")

def digest(path):
    value = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1 << 20), b""):
            value.update(block)
    return value.hexdigest()

def build_image(root, stack, packer):
    subprocess.run([sys.executable, str(packer), "--stack", stack, "--output", str(root)], check=True)

def image_digest(cli, image):
    result = subprocess.run([str(cli), "image-digest", "--path", str(image)], check=True, capture_output=True, text=True)
    value = result.stdout.strip()
    if len(value) != 71 or not value.startswith("sha256:") or any(c not in "0123456789abcdef" for c in value[7:]):
        fail("runner CLI returned an invalid image digest for " + str(image))
    return value

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", required=True)
    parser.add_argument("--runner-cli", default=str(Path(__file__).resolve().parent.parent / "bin/reforge-runner"))
    parser.add_argument("--runsc", required=True)
    parser.add_argument("--tool", required=True)
    parser.add_argument("--state-root", required=True)
    parser.add_argument("--cgroup-root")
    parser.add_argument("--stack", action="append", choices=("go", "javascript", "python"))
    parser.add_argument("--development", action="store_true")
    parser.add_argument("--rootless", action="store_true")
    parser.add_argument("--memory-bytes", type=int, default=512 << 20)
    parser.add_argument("--disk-bytes", type=int, default=256 << 20)
    parser.add_argument("--cpus", type=int, default=2)
    parser.add_argument("--max-processes", type=int, default=256)
    args = parser.parse_args()
    stacks = list(dict.fromkeys(args.stack or ("go", "javascript", "python")))
    output = absolute(args.output, "output")
    state = absolute(args.state_root, "state root")
    no_symlink_components(output.parent, "output parent")
    no_symlink_components(state.parent, "state parent")
    if output.exists() or output.is_symlink() or state.exists() or state.is_symlink():
        fail("output and state root must not already exist")
    if output == state or output in state.parents or state in output.parents:
        fail("output and state root must not be nested")
    cli = absolute(args.runner_cli, "runner CLI")
    runsc = absolute(args.runsc, "runsc")
    tool = absolute(args.tool, "sandbox tool")
    packer = Path(__file__).resolve().parent / "build-runner-images.py"
    private_regular(cli, "runner CLI")
    private_regular(runsc, "runsc")
    private_regular(tool, "sandbox tool")
    no_symlink_components(packer, "image packer")
    if not packer.is_file() or packer.stat().st_mode & 0o022 or packer.parent.stat().st_mode & 0o022:
        fail("image packer is unavailable or writable")
    if args.development and args.cgroup_root:
        fail("development mode must not configure a cgroup root")
    cgroup = None
    if not args.development:
        if not args.cgroup_root:
            fail("production mode requires --cgroup-root")
        cgroup = absolute(args.cgroup_root, "cgroup root")
        if not str(cgroup).startswith("/sys/fs/cgroup/"):
            fail("production cgroup root must be under /sys/fs/cgroup")
        private_directory(cgroup, "cgroup root")
    if args.memory_bytes < 256 << 20 or args.memory_bytes > 64 << 30 or args.disk_bytes < 16 << 20 or args.disk_bytes > args.memory_bytes // 2 or args.cpus < 1 or args.cpus > 32 or args.max_processes < 32 or args.max_processes > 4096:
        fail("resource limits are outside the runtime boundary")
    output.mkdir(mode=0o700, parents=False)
    created_state = False
    try:
        images = {}
        recipes = {}
        for stack in stacks:
            image = output / ("image-" + stack)
            build_image(image, stack, packer)
            value = image_digest(cli, image)
            images[value] = str(image)
            recipes[stack] = value
        state.mkdir(mode=0o700, parents=False)
        created_state = True
        config = {"runsc": str(runsc), "runsc_sha256": digest(runsc), "tool": str(tool), "tool_sha256": digest(tool), "state_root": str(state), "cgroup_root": str(cgroup) if cgroup else "", "images": images, "development": args.development, "rootless": args.rootless, "memory_bytes": args.memory_bytes, "disk_bytes": args.disk_bytes, "cpus": args.cpus, "max_processes": args.max_processes}
        config_path = output / "runtime-config.json"
        fd = os.open(config_path, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
        with os.fdopen(fd, "w", encoding="utf-8") as stream:
            json.dump(config, stream, sort_keys=True, indent=2)
            stream.write("\n")
        print("runtime_config=" + str(config_path))
        print("REFORGE_REPAIR_IMAGES=" + json.dumps(recipes, sort_keys=True, separators=(",", ":")))
    except BaseException:
        if created_state and state.exists():
            state.rmdir()
        if output.exists():
            shutil.rmtree(output)
        raise

if __name__ == "__main__":
    main()
