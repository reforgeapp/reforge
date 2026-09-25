#!/usr/bin/env python3
import argparse
import hashlib
import os
import re
import shutil
import stat
import subprocess
import sys

MOUNTS = ("proc", "dev", "tmp", "home", "workspace", "opt", "opt/reforge", "opt/deps")
SKIP_DIRS = {"__pycache__", "site-packages", "test", "tests", "doc", "docs"}


def fail(message):
    raise RuntimeError(message)


def command_path(command):
    result = shutil.which(command)
    if not result:
        fail("required tool not found: " + command)
    return regular_source(result, executable=True)


def regular_source(source, executable=False):
    source = os.path.realpath(source)
    info = os.stat(source)
    if not stat.S_ISREG(info.st_mode) or info.st_mode & 0o022 or (executable and info.st_mode & 0o111 == 0):
        fail("tool source is not a trusted regular file: " + source)
    return source


def destination(root, absolute):
    target = os.path.join(root, os.path.normpath(absolute).lstrip("/"))
    os.makedirs(os.path.dirname(target), mode=0o755, exist_ok=True)
    return target


def copy_file(root, source, absolute):
    source = regular_source(source)
    target = destination(root, absolute)
    if os.path.exists(target):
        if file_digest(source) != file_digest(target):
            fail("conflicting dependency destination: " + absolute)
        return
    shutil.copyfile(source, target)
    os.chmod(target, stat.S_IMODE(os.stat(source).st_mode) & 0o755)


def file_digest(filename):
    digest = hashlib.sha256()
    with open(filename, "rb") as stream:
        for block in iter(lambda: stream.read(1 << 20), b""):
            digest.update(block)
    return digest.digest()


def copy_tree(root, source, absolute, skip_nested=True):
    source = os.path.realpath(source)
    if not os.path.isdir(source):
        fail("toolchain directory missing: " + source)
    for current, dirs, files in os.walk(source, followlinks=False):
        if any(os.path.islink(os.path.join(current, directory)) for directory in dirs):
            fail("toolchain directory symlink is not allowed: " + current)
        relative = os.path.relpath(current, source)
        dirs[:] = sorted(d for d in dirs if d not in SKIP_DIRS or not skip_nested and relative != ".")
        files.sort()
        for filename in files:
            source_file = os.path.join(current, filename)
            if filename.lower() == "readme.md" or filename.endswith(".pyc") or filename.endswith(".pyo") or filename.endswith("_test.go"):
                continue
            rel = filename if relative == "." else os.path.join(relative, filename)
            copy_file(root, source_file, os.path.join(absolute, rel))


def linked_libraries(binary, interpreter=None):
    if interpreter:
        result = subprocess.run([interpreter, "--list", binary], check=False, capture_output=True, text=True)
    else:
        ldd = command_path("ldd")
        result = subprocess.run([ldd, binary], check=False, capture_output=True, text=True)
    output = result.stdout + "\n" + result.stderr
    if result.returncode != 0 and "not a dynamic executable" not in output and "statically linked" not in output:
        fail("ldd failed for trusted binary: " + binary)
    if "=> not found" in output:
        fail("ldd reported missing dependency for trusted binary: " + binary)
    paths = set()
    for line in output.splitlines():
        match = re.search(r"=>\s+(/[^\s]+)|^\s*(/[^\s]+)\s+\(", line)
        if match:
            paths.add(match.group(1) or match.group(2))
    return sorted(paths)


def elf_interpreter(binary):
    readelf = command_path("readelf")
    result = subprocess.run([readelf, "-l", binary], check=True, capture_output=True, text=True)
    match = re.search(r"Requesting program interpreter:\s+([^]]+)", result.stdout)
    return match.group(1).strip() if match else None


def copy_binary_dependencies(root, binary, main_interpreter=None):
    interpreter = elf_interpreter(binary) or main_interpreter
    if interpreter:
        interpreter_source = regular_source(interpreter, executable=True)
        copy_file(root, interpreter_source, interpreter)
    for library in linked_libraries(binary, interpreter):
        copy_file(root, library, library)
        if interpreter and "/opt/glibc/" in library:
            copy_file(root, library, os.path.join(os.path.dirname(interpreter), os.path.basename(library)))
        if library.startswith("/lib/"):
            copy_file(root, library, "/lib64/" + os.path.basename(library))
            if interpreter:
                copy_file(root, library, os.path.join(os.path.dirname(interpreter), os.path.basename(library)))


def go_toolchain(root, binary):
    goroot = subprocess.run([binary, "env", "GOROOT"], check=True, capture_output=True, text=True).stdout.strip()
    if not os.path.isabs(goroot):
        fail("Go GOROOT is not absolute")
    tool_binary = regular_source(os.path.join(goroot, "bin", "go"), executable=True)
    copy_tree(root, goroot, "/usr/local/go", skip_nested=False)
    copy_binary_dependencies(root, tool_binary)
    copy_file(root, tool_binary, "/usr/local/go/bin/go")
    version = subprocess.run([tool_binary, "version"], check=True, capture_output=True, text=True).stdout.strip()
    return version


def node_toolchain(root, binary):
    copy_file(root, binary, "/usr/local/bin/node")
    copy_binary_dependencies(root, binary)
    return subprocess.run([binary, "--version"], check=True, capture_output=True, text=True).stdout.strip()


def python_toolchain(root, binary):
    stdlib = subprocess.run([binary, "-c", "import sysconfig; print(sysconfig.get_path('stdlib'))"], check=True, capture_output=True, text=True).stdout.strip()
    if not os.path.isabs(stdlib):
        fail("Python stdlib path is not absolute")
    copy_tree(root, stdlib, "/usr/local/lib/" + os.path.basename(stdlib))
    interpreter = elf_interpreter(binary)
    for current, dirs, files in os.walk(stdlib, followlinks=False):
        if any(os.path.islink(os.path.join(current, directory)) for directory in dirs):
            fail("Python stdlib directory symlink is not allowed: " + current)
        dirs[:] = sorted(d for d in dirs if d not in SKIP_DIRS)
        for filename in files:
            if filename.endswith(".so") or ".so." in filename:
                copy_binary_dependencies(root, os.path.join(current, filename), interpreter)
    copy_file(root, binary, "/usr/local/bin/python3")
    copy_binary_dependencies(root, binary)
    return subprocess.run([binary, "--version"], check=True, capture_output=True, text=True).stdout.strip()


def make_layout(root):
    for mount in MOUNTS:
        os.makedirs(os.path.join(root, mount), mode=0o755, exist_ok=True)
    tool = os.path.join(root, "opt/reforge/tool")
    with open(tool, "wb"):
        pass
    os.chmod(tool, 0o555)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--stack", choices=("go", "javascript", "python"), required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    output = os.path.abspath(args.output)
    if not os.path.isabs(args.output) or os.path.lexists(output):
        fail("output must be an absolute path that does not exist")
    os.makedirs(output, mode=0o755)
    try:
        make_layout(output)
        binary = command_path({"go": "go", "javascript": "node", "python": "python3"}[args.stack])
        if args.stack == "go": version = go_toolchain(output, binary)
        elif args.stack == "javascript": version = node_toolchain(output, binary)
        else: version = python_toolchain(output, binary)
        print("imagepath=" + output)
        print("toolchainversion=" + version)
    except Exception:
        shutil.rmtree(output)
        raise


if __name__ == "__main__":
    try:
        main()
    except (OSError, subprocess.CalledProcessError, RuntimeError) as error:
        print("build-runner-images: " + str(error), file=sys.stderr)
        raise SystemExit(1)
