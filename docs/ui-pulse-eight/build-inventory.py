"""Inventory a frozen capture stage; compare same-size PNGs for near duplicates."""
from pathlib import Path
from hashlib import sha256
import json
import sys
from PIL import Image, ImageChops

root = Path(__file__).resolve().parent
stage, version = sys.argv[1:]
manifest_path = root / f'manifest-{stage}-after.json'
manifest = json.loads(manifest_path.read_text())
rows, seen = [], {}
for shot in sorted(manifest['shots'], key=lambda row:row['file']):
    file = root/'screenshots'/shot['file']
    data = file.read_bytes()
    with Image.open(file) as im:
        width,height=im.size
    assert sha256(data).hexdigest() == shot['sha256'] and len(data)==shot['bytes']
    assert (width,height)==(shot['viewport']['width'],shot['viewport']['height'])
    rows.append(dict(file='screenshots/'+file.name,sha256=shot['sha256'],bytes=len(data),width=width,height=height,surface=shot['surface'],cohort='settings' if shot['surface'] in ['settings','backup','appearance'] else 'admin-console',state='empty/unpaired' if shot['surface'] in ['activity','logs'] else 'synthetic sparse/default',theme=shot['theme_source'],duplicate_of=seen.get(shot['sha256'])))
    seen.setdefault(shot['sha256'],'screenshots/'+file.name)
pairs=[]
for i,a in enumerate(rows):
    with Image.open(root/a['file']) as im:
        source=im.convert('RGB')
    for b in rows[i+1:]:
        if (a['width'],a['height'])!=(b['width'],b['height']):continue
        with Image.open(root/b['file']) as im:
            diff=ImageChops.difference(source,im.convert('RGB'))
        maximum=max(hi for _,hi in diff.getextrema())
        if maximum<=8:
            pixels=list(diff.getdata());fraction=sum(any(v) for v in pixels)/len(pixels)
            if fraction<.001:
                pairs.append(dict(a=a['file'],b=b['file'],different_pixel_fraction=fraction,max_channel_delta=maximum))
                b['duplicate_of']=b['duplicate_of'] or a['file']
(root/f'inventory-v{version}.json').write_text(json.dumps(rows,indent=2)+'\n')
(root/f'duplicate-audit-v{version}.json').write_text(json.dumps(dict(rule='same dimensions, changed pixels<0.1%, max channel delta<=8',near_duplicates=pairs),indent=2)+'\n')
print(f'{len(rows)} images; {len(seen)} exact groups; {len(pairs)} near-duplicate pairs')
