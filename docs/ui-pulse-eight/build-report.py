"""Validate immutable Council inputs and export independent cohort scores."""
from pathlib import Path
from decimal import Decimal, ROUND_HALF_UP
from xml.sax.saxutils import escape
import csv
import hashlib
import json
import re
import struct
import zipfile
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parent
WEIGHTS = dict(hierarchy='.20', typography='.15', density_layout='.20', color='.15', visible_affordances='.15', coherence='.15')
NS = 'http://schemas.openxmlformats.org/spreadsheetml/2006/main'

def rounded(value):
    return value.quantize(Decimal('.1'), rounding=ROUND_HALF_UP)

def normalized(file):
    return file.removeprefix('docs/ui-pulse-eight/')

def checked_image(row):
    data = (ROOT / normalized(row['file'])).read_bytes()
    assert hashlib.sha256(data).hexdigest() == row['sha256'], row['file']
    assert len(data) == row['bytes'], row['file']
    assert struct.unpack('>II', data[16:24]) == (row['width'], row['height']), row['file']

inventories = {}
evidence = [['Inventory', 'File', 'SHA256', 'Bytes', 'Width', 'Height', 'Cohort', 'State', 'Duplicate of']]
for path in sorted(ROOT.glob('inventory-v*.json')):
    rows = json.loads(path.read_text())
    inventories[path.name] = rows
    for row in rows:
        checked_image(row)
        evidence.append([path.name, *[row.get(k) for k in ['file','sha256','bytes','width','height','cohort','state','duplicate_of']]])

round_rows = [['Round','Reviewer','Cohort','Unique images','Exact mean','Displayed mean','At least 8','Reviewer satisfied','Report SHA256']]
score_rows = [['Round','Reviewer','File','Cohort',*WEIGHTS,'Quality','Confidence','Caveat']]
spread_rows = [['Round','File','Dimension','Opus','Sol','Spread']]
brand_keys = ['palette_tokens','type_geometry','shell','accent_discipline','suite_cohesion']
brand_rows = [['Round','Reviewer','File',*brand_keys,'Separate brand mean']]
reports = {}
for path in sorted(ROOT.glob('*-round[0-9]*.json')):
    match = re.fullmatch(r'(opus|sol|gemini)-round(\d+)\.json', path.name)
    if not match:
        continue
    reviewer, number = match[1], int(match[2])
    report = json.loads(path.read_text())
    assert report['schema'] == 'ky-ui-council-scores/2'
    assert report['rubric']['version'] == 'ky-suite-v2'
    assert {k:Decimal(str(v)) for k,v in report['rubric']['weights'].items()} == {k:Decimal(v) for k,v in WEIGHTS.items()}
    files = report['files']
    assert len({normalized(f['file']) for f in files}) == len(files)
    expected = next(rows for rows in inventories.values() if {normalized(r['file']) for r in rows} == {normalized(f['file']) for f in files})
    pins = {normalized(row['file']):row for row in expected}
    no_score = report.get('verdict', {}).get('status') == 'no_visual_score_tool_limitation'
    if no_score:
        assert report['verdict']['satisfied'] is None
    groups = {}
    for f in files:
        pin = pins[normalized(f['file'])]
        assert all(f[k] == pin[k] for k in ['sha256','width','height']), f['file']
        if no_score:
            assert all(f['scores'][k] is None for k in WEIGHTS) and f['total'] is None
            assert all(f['ky_fit']['scores'][k] is None for k in brand_keys)
            assert f['ky_fit']['mean'] is None
            if not f.get('duplicate_of'):
                groups.setdefault(f['cohort'], []).append(None)
            score_rows.append([number,reviewer,normalized(f['file']),f['cohort'],*[None]*7,f.get('confidence'),f.get('caveat')])
            brand_rows.append([number,reviewer,normalized(f['file']),*[None]*6])
            continue
        assert all(type(f['scores'][k]) is int and 0 <= f['scores'][k] <= 10 for k in WEIGHTS)
        total = rounded(sum(Decimal(WEIGHTS[k])*f['scores'][k] for k in WEIGHTS))
        assert total == Decimal(str(f['total'])), f['file']
        if not f.get('duplicate_of'):
            groups.setdefault(f['cohort'], []).append(total)
        fit = f.get('ky_fit', {})
        fit_scores = fit.get('scores', fit)
        fit_values = [fit_scores.get(key) for key in brand_keys]
        numeric = [Decimal(str(value)) for value in fit_values if isinstance(value, (int,float))]
        brand_rows.append([number,reviewer,normalized(f['file']),*fit_values,float(rounded(sum(numeric)/len(numeric))) if numeric else None])
        score_rows.append([number,reviewer,normalized(f['file']),f['cohort'],*[f['scores'][k] for k in WEIGHTS],float(total),f.get('confidence'),f.get('caveat')])
    means = {} if no_score else {cohort:sum(values)/len(values) for cohort,values in groups.items()}
    reports[(reviewer,number)] = dict(report=report,means=means,files={normalized(f['file']):f for f in files})
    if no_score:
        for cohort,values in groups.items():
            round_rows.append([number,reviewer,cohort,len(values),None,None,None,'No score',hashlib.sha256(path.read_bytes()).hexdigest()])
    for cohort,mean in means.items():
        round_rows.append([number,reviewer,cohort,len(groups[cohort]),float(mean),float(rounded(mean)),mean>=8,report.get('verdict',{}).get('satisfied'),hashlib.sha256(path.read_bytes()).hexdigest()])
for number in sorted({number for _,number in reports}):
    a,b = reports.get(('opus',number)),reports.get(('sol',number))
    if a and b:
        assert a['files'].keys() == b['files'].keys()
        for file in a['files']:
            for key in WEIGHTS:
                x,y = a['files'][file]['scores'][key],b['files'][file]['scores'][key]
                spread_rows.append([number,file,key,x,y,abs(x-y)])

notes = [['Topic','Meaning'],['Gate','Every covered cohort must have exact mean >=8 and both primaries satisfied; no cross-cohort or reviewer averaging.'],['Rounding','Per image: exact fixed-weight sum, half-up to one decimal. Means use those totals; displayed mean is half-up to one decimal.'],['Scope','Synthetic admin, initial viewport. Rounds1–6:20admin-console/12settings. Round7 onward adds expanded Logs/Activity filters:28admin-console/12settings. Logs/Activity empty/unpaired; full product acceptance unverified.'],['Sol round1 caveat','All32 originals viewed first; four Status dimension rows transcribed after history. Even setting every Status picture to10 gives admin mean7.82.'],['Spread audit','Round1 has25 views with at least one dimension spread>=2. Parent re-inspected the full contact sheet and representative original Logs/Settings/Alerts/Backup views. Independent judgments retained; supported defects corrected.'],['Gemini','Deferred until both primaries satisfied. Advisory ratings require original image access; parent audits claims.'],['Interim stage01','Frozen after captures show0.4.2. Round2 uses stage02 after the0.4.3 warning fix. Stage01 is unscored iteration evidence.'],['Historical scores','Earlier Pulse6.9 coverage differs; no controlled per-file drift.'],['MySlop','HTTP401 unknown or revoked token; no shared-board handoff posted.']]
notes.append(['Focused fix confirmation','Explicitly unblinded: each reviewer pins its immediately preceding report, carries unaffected dimensions forward and reassesses actual patch effects/regressions after viewing every current original. This is not a fresh independent ranking; see the report method and baseline pins.'])
notes.append(['Gemini round14','No visual score: image attachments were unavailable in its ACP harness. Null scores are preserved, never zero-filled or treated as approval. See round14-gemini-audit.json for rejected metadata claims and scoped native verification.'])
audit_rows = [['Audit','Evidence']]
for audit_path in sorted(ROOT.glob('round*-audit.json')):
    audit = json.loads(audit_path.read_text())
    for file,pin in audit.get('raw_reports', {}).items():
        assert hashlib.sha256((ROOT/file).read_bytes()).hexdigest() == pin, file
    audit_rows.append([audit_path.name,json.dumps(audit,ensure_ascii=False)])
checks = [['Item','Evidence']]
check_path = ROOT/'verification.json'
if check_path.exists():
    checks += [[k,json.dumps(v,ensure_ascii=False)] for k,v in json.loads(check_path.read_text()).items()]
prs = [['Repository','PR','Status']]
if (ROOT/'delivery.json').exists():
    prs += [[r['repository'],r['url'],r['status']] for r in json.loads((ROOT/'delivery.json').read_text())]

markdown = ['# KyPulse Council score history','','Independent visual scores; settings and admin-console stay separate. A displayed8.0 does not clear an exact mean below8.','','| Round | Reviewer | Admin console | Settings | Satisfied |','| --- | --- | ---: | ---: | --- |']
latest_round = max([1,*[n for _,n in reports],*[json.loads(path.read_text()).get('councilRound',1) for path in ROOT.glob('manifest-*.json')]])
for number in range(1,latest_round+1):
    for name,label in [('opus','Claude Opus5.5'),('sol','GPTSol'),('gemini','Gemini4Argon')]:
        r=reports.get((name,number))
        vals=[(f"{r['means'][c]:.4f} (<8)" if r['means'][c] < 8 <= rounded(r['means'][c]) else str(rounded(r['means'][c]))) if r and c in r['means'] else ('No score' if r else 'Deferred' if name=='gemini' else 'Pending') for c in ['admin-console','settings']]
        satisfied=('No score' if r['report'].get('verdict',{}).get('satisfied') is None else str(r['report']['verdict']['satisfied'])) if r else '—'
        markdown.append(f'| {number} | {label} | {vals[0]} | {vals[1]} | {satisfied} |')
markdown += ['', 'Reports labelled focused fix confirmation are explicitly unblinded: unaffected dimensions come from the same reviewer’s pinned immediately preceding round; actual changes and regressions are reassessed. They are not fresh independent rankings.']
markdown += ['', 'Baseline exact means: Opus6.28/6.95; Sol7.39/8.00. The Sol Status transcription caveat remains in its raw report. Parent reinspection retained independent scores. No provider-level or cross-cohort mean is used.','','See KyPulse-Council.xlsx for dimensions, hashes, evidence, spreads and checks. Original reviewer reports remain unchanged.']
(ROOT/'COUNCIL.md').write_text('\n'.join(markdown)+'\n')
for name,rows in [('rounds',round_rows),('scores',score_rows),('spreads',spread_rows)]:
    with (ROOT/f'{name}.csv').open('w',newline='') as out:
        csv.writer(out, lineterminator='\n').writerows(rows)

def column(index):
    result = ""
    while index:
        index, rem = divmod(index - 1, 26)
        result = chr(65 + rem) + result
    return result

def sheet(rows):
    widths = [min(70, max(12, max(len(str(row[c])) if c < len(row) and row[c] is not None else 0 for row in rows) + 2)) for c in range(max(map(len, rows)))]
    cols = "".join(f'<col min="{i}" max="{i}" width="{width}" customWidth="1"/>' for i, width in enumerate(widths, 1))
    out = [f'<worksheet xmlns="{NS}"><sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews><cols>{cols}</cols><sheetData>']
    for r, row in enumerate(rows, 1):
        out.append(f'<row r="{r}">')
        for c, value in enumerate(row, 1):
            ref = f"{column(c)}{r}"
            if value is None:
                continue
            if isinstance(value, (int, float)) and not isinstance(value, bool):
                out.append(f'<c r="{ref}"><v>{value}</v></c>')
            else:
                out.append(f'<c r="{ref}" t="inlineStr"><is><t xml:space="preserve">{escape(str(value))}</t></is></c>')
        out.append("</row>")
    out.append(f'</sheetData><autoFilter ref="A1:{column(max(map(len, rows)))}{len(rows)}"/></worksheet>')
    return "".join(out)

sheets = [("Rounds", round_rows), ("Scores", score_rows), ("Brand fit", brand_rows), ("Audits", audit_rows), ("Evidence", evidence), ("Spreads", spread_rows), ("Checks", checks), ("Read me", notes), ("Pull requests", prs)]
workbook = ROOT / "KyPulse-Council.xlsx"
with zipfile.ZipFile(workbook, "w", zipfile.ZIP_DEFLATED) as z:
    types = ['<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>']
    for i in range(1, len(sheets) + 1):
        types.append(f'<Override PartName="/xl/worksheets/sheet{i}.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>')
    z.writestr("[Content_Types].xml", "".join(types) + "</Types>")
    z.writestr("_rels/.rels", '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>')
    z.writestr("xl/workbook.xml", f'<workbook xmlns="{NS}" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>' + "".join(f'<sheet name="{name}" sheetId="{i}" r:id="rId{i}"/>' for i, (name, _) in enumerate(sheets, 1)) + "</sheets></workbook>")
    z.writestr("xl/_rels/workbook.xml.rels", '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">' + "".join(f'<Relationship Id="rId{i}" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet{i}.xml"/>' for i in range(1, len(sheets) + 1)) + "</Relationships>")
    for i, (_, rows) in enumerate(sheets, 1):
        z.writestr(f"xl/worksheets/sheet{i}.xml", sheet(rows))
with zipfile.ZipFile(workbook) as z:
    assert z.testzip() is None
    for file in z.namelist():
        ET.fromstring(z.read(file))

print("Validated reports, arithmetic, evidence and workbook XML")
