import json
import pathlib
import subprocess

root = pathlib.Path(__file__).resolve().parents[1]

def modules_from(command):
    raw = subprocess.check_output(command, cwd=root, text=True)
    decoder = json.JSONDecoder()
    result = []
    while raw.strip():
        raw = raw.lstrip()
        item, end = decoder.raw_decode(raw)
        result.append(item)
        raw = raw[end:]
    return result

def used_modules(command):
    used = set()
    for package in modules_from(command):
        module = package.get('Module') or {}
        if module.get('Path') and module.get('Version'):
            used.add((module['Path'], module['Version']))
    return used

def escaped_module_part(value):
    return ''.join('!' + char.lower() if char.isupper() else char for char in value)

def notice_files(directory):
    if directory is None or not directory.is_dir():
        return []
    return sorted(
        path for path in directory.iterdir()
        if path.is_file() and path.name.lower().startswith(
            ('license', 'licence', 'copying', 'notice')
        )
    )

modules = modules_from(['go', 'list', '-m', '-json', 'all'])
module_cache = pathlib.Path(subprocess.check_output(['go', 'env', 'GOMODCACHE'], cwd=root, text=True).strip())
runtime = used_modules([
    'go', 'list', '-deps', '-json',
    './cmd/server', './cmd/migrate', './cmd/evidence', './cmd/runner',
])
tests = used_modules(['go', 'list', '-deps', '-test', '-json', './...'])
rows = []
notices = []
for module in modules:
    if module.get('Main'):
        continue
    path, version = module['Path'], module['Version']
    key = (path, version)
    cache_path = escaped_module_part(path) + '@' + escaped_module_part(version)
    directory = module_cache / cache_path
    if not directory.is_dir() and module.get('Dir'):
        directory = pathlib.Path(module['Dir'])
    evidence = notice_files(directory)
    scope = []
    if key in runtime:
        scope.append('runtime binary')
    if key in tests:
        scope.append('Go tests')
    if not scope:
        scope.append('module graph/build download only')
    evidence_text = ', '.join(file.name for file in evidence) if evidence else 'unavailable; source not cached'
    rows.append((path, version, '; '.join(scope), evidence_text))
    for file in evidence:
        text = '\n'.join(line.rstrip() for line in file.read_text(errors='replace').splitlines()).rstrip()
        notices.append(f'{path} {version} / {file.name}\n{text}')

lock = json.loads((root / 'web/package-lock.json').read_text())
npm_packages = []
for package_path, item in sorted(lock['packages'].items()):
    if not package_path:
        continue
    name = package_path.removeprefix('node_modules/')
    version = item.get('version', 'unknown')
    directory = root / 'web' / package_path
    package_json = directory / 'package.json'
    metadata = json.loads(package_json.read_text()) if package_json.is_file() else {}
    repository = metadata.get('repository') or {}
    repository_url = repository if isinstance(repository, str) else repository.get('url', '')
    repository_url = repository_url.strip().lower().removeprefix('git+').replace('git://', 'https://').removesuffix('.git').rstrip('/')
    repository_dir = repository.get('directory', '') if isinstance(repository, dict) else ''
    evidence = notice_files(directory)
    npm_packages.append((name, version, item, directory, metadata, repository_url, repository_dir, evidence))

npm_sources = {}
for name, version, item, directory, metadata, repository_url, repository_dir, evidence in npm_packages:
    licence_files = [file for file in evidence if file.name.lower().startswith(('license', 'licence', 'copying'))]
    declared = item.get('license', 'no package metadata licence')
    if repository_url and licence_files:
        key = (version, repository_url, repository_dir, declared)
        npm_sources.setdefault(key, (name, licence_files))

for name, version, item, directory, metadata, repository_url, repository_dir, evidence in npm_packages:
    scope = 'frontend build/test only' if item.get('dev', False) else 'frontend bundle'
    declared = item.get('license', 'no package metadata licence')
    if evidence:
        evidence_text = ', '.join(file.name for file in evidence)
    else:
        key = (version, repository_url, repository_dir, declared)
        inherited = npm_sources.get(key) if repository_url else None
        if inherited:
            source_name, source_files = inherited
            evidence_text = ', '.join(
                f'{file.name} via {source_name}@{version} (matching repository, directory, version, and declared licence)'
                for file in source_files
            )
        else:
            evidence_text = 'no top-level licence/notice file'
    rows.append((name, version, scope, f'{declared}; {evidence_text}'))
    for file in evidence:
        text = '\n'.join(line.rstrip() for line in file.read_text(errors='replace').splitlines()).rstrip()
        notices.append(f'{name} {version} / {file.name}\n{text}')

inventory = [
    '# Dependency inventory',
    '',
    'Generated from the Go module graph and npm lockfile. Go package scope comes from',
    '`go list -deps` for shipped binaries and `go list -deps -test ./...`; npm scope',
    'comes from each locked package’s production/development install role. Exact-root',
    'licence and notice files are copied to `third-party-notices.txt` with trailing whitespace normalized. The Go',
    'licence name is not inferred from copyright text.',
    '',
    '| Dependency | Version | Scope | Licence evidence |',
    '| --- | --- | --- | --- |',
]
inventory.extend(
    '| ' + ' | '.join(value.replace('|', '\\|') for value in row) + ' |'
    for row in rows
)
(root / 'docs/implementation/dependencies.md').write_text('\n'.join(inventory) + '\n')
(root / 'docs/implementation/third-party-notices.txt').write_text(
    '\n\n'.join(notices).rstrip() + '\n'
)
