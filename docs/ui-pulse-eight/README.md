# KyPulse visual8 progression

Round1 is the fresh baseline at clean product source fef7b670a36d48792137f56d839e49a86d44eadc. Product UI has not changed. Historical6.9 has different coverage and is not a controlled baseline for this set.

| Round | Opus5.5 | GPTSol | Gemini4Argon |
| --- | --- | --- | --- |
|1,baseline|Pending|Pending|Deferred|

Source: manifest-00-before.json and inventory-v1.json;32distinct captures,20admin-console and12settings. Eight surfaces:Status,Alerts,appdetail,Logs,Activity,Settings,Appearance,Backup. Logs/Activity are empty and unpaired,not populated-state proof. Initial viewport only; lower content remains outside captured coverage.

Capture conditions: Chromium153.0.8010.12,scale1,normal service workers,1280x900/390x844,OSlight/dark,no savedthemechoice; synthetic admin and3watched apps (ok,degraded,down) from an owned private-interface fake upstream. The compiled server embeds committed web/dist; npmci and Go build passed. Vendored ky-ui is0.4.1 at baseline. No application behavior/full-browser suite is claimed from capture-only execution.

The first capture attempt failed on the test selector Backup; the actual heading is Backup & recovery. The corrected second attempt captured32views. This was a capture-driver correction,not a product change. /tmp/kypulse-eight-baseline-capture{-final,}.log records both attempts.

KEEP_FIXTURE=1 keeps the capture-owned http://127.0.0.1:18420 server alive for Computer Use until SIGTERM. Disposable admin credentials are admin / CaptureUpdated456!; fixture.json is ignored and contains only synthetic state. Never point the helper at real data or disable CSP/CSRF/service workers. Later implementation changes require the product's full checks.

Read workspace,product,web and browser AGENTS before editing. The parent owns score tables and a spreadsheet after terminal reviews; each delegate owns only its report pair. Both primaries must be satisfied before Gemini; parent-audit its findings and retain raw reports. One left navigation region is mandatory. No merge or deployment is authorized.
