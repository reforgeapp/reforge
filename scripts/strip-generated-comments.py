from pathlib import Path
import re

files = [*Path('pkg/httpapi/generated').glob('*.go'), *Path('pkg/store/dbgen').glob('*.go'), Path('web/src/api/schema.ts')]
for file in files:
    text = file.read_text()
    text = re.sub(r'/\*.*?\*/', '', text, flags=re.S)
    text = re.sub(r'^\s*//[^\n]*\n', '', text, flags=re.M)
    file.write_text('\n'.join(line.rstrip() for line in text.splitlines()).strip() + '\n')
