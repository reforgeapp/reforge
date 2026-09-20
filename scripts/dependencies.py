import json, pathlib, subprocess
root=pathlib.Path(__file__).resolve().parents[1]
raw=subprocess.check_output(['go','list','-m','-json','all'],cwd=root,text=True)
mods=[];decoder=json.JSONDecoder()
while raw.strip():
 m,n=decoder.raw_decode(raw.lstrip());mods.append(m);raw=raw.lstrip()[n:]
rows=[];notices=[]
for m in mods:
 if m.get('Main'):continue
 directory=pathlib.Path(m['Dir']) if 'Dir' in m else None
 files=[] if directory is None else [p for p in directory.iterdir() if p.is_file() and p.name.lower().startswith(('license','licence','copying','notice'))]
 rows.append((m['Path'],m['Version'],'included' if files else 'review required'))
 for f in files:notices.append(m['Path']+' '+m['Version']+' / '+f.name+'\n'+f.read_text(errors='replace'))
pkg=json.loads((root/'web/package-lock.json').read_text())
for name,item in pkg['packages'].items():
 if not name:continue
 rows.append((name.removeprefix('node_modules/'),item['version'],item.get('license','review required')))
 directory=root/'web'/name
 if directory.exists():
  for f in directory.iterdir():
   if f.is_file() and f.name.lower().startswith(('license','licence','copying','notice')):notices.append(name+' '+item['version']+' / '+f.name+'\n'+f.read_text(errors='replace'))
(root/'docs/implementation/dependencies.md').write_text('# Dependency inventory\n\nGenerated from pinned Go modules and npm lockfile. Includes development tools. Full available notices: `third-party-notices.txt`. T28 reviews distribution-specific coverage.\n\n| Dependency | Version | Licence evidence |\n| --- | --- | --- |\n'+''.join('| '+ ' | '.join(row)+' |\n' for row in rows))
(root/'docs/implementation/third-party-notices.txt').write_text('\n'.join(line.rstrip() for line in '\n\n'.join(notices).splitlines()).rstrip()+'\n')
