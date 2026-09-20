from pathlib import Path
import re

files = [*Path('internal/httpapi/generated').glob('*.go'), *Path('internal/store/dbgen').glob('*.go'), Path('web/src/api/schema.ts')]
for file in files:
    text = file.read_text()
    text = re.sub(r'/\*.*?\*/', '', text, flags=re.S)
    text = re.sub(r'^\s*//[^\n]*\n', '', text, flags=re.M)
    file.write_text('\n'.join(line.rstrip() for line in text.splitlines()).strip() + '\n')
