# KyPulse sequential UI evidence

## Purpose

- Keep source captures, independent ratings and verification for the owner’s visual8 progression gate.

## Ownership

- capture.mjs owns the disposable synthetic capture fixture; screenshots and manifests remain frozen by stage.
- build-inventory.py owns per-stage PNG hashing and near-duplicate checks (Python3 with Pillow); build-report.py validates fixed-rubric reports and exports the workbook/CSVs using the standard library.
- Each delegate writes only its own report. The parent owns inventories,score tables,spreadsheet and audits.

## Local Contracts

- Use one left navigation region; preserve routes,roles,CSP,CSRF,themes and native semantics.
- Read original images at full resolution before numeric priors; record hashes,viewport,theme,state and fixedky-suite-v2 arithmetic.
- Keep comparable cohorts separate. Unseen behavior stays unverified; a screenshot target is not an achieved score.
- Opus5.5 and GPTSol review first; Gemini4Argon is advisory only after both satisfied,then parent-audited. Show every round’s scores including deferred reviewers.
- Wait for terminal reviews before regenerating shared score artifacts. Keep original reports unchanged; corrections belong in explicit audits.

## Work Guidance

- Use Computer Use on an owned fixture/tab with synthetic data only. No deployment or merge is authorized.
- Capture1280x900/390x844,light/dark with normal service-worker behavior. Logs/Activity baseline are empty/unpaired; do not imply populated-state coverage.

## Verification

- Build .browser/server from this tree,then node docs/ui-pulse-eight/capture.mjs <nn> before|after. It refuses an existing manifest. KEEP_FIXTURE=1 retains its owned server for native review until SIGTERM; otherwise cleanup runs after capture.
- Run python3 docs/ui-pulse-eight/build-report.py only after every review in the round is terminal. It validates original hashes, inventory membership, arithmetic, audit input pins and workbook XML.
- Run the existing product/browser checks required by parent AGENTS for implementation changes. Capture-only runs establish provenance,not behavioral acceptance.

## Child DOX Index

- None.
