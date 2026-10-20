# Container-local installer setup

Run `kypulse apply-setup --capabilities` to discover the source interface.
`apply-setup --file /absolute/private/bundle.json [--check]` uses the shared
`ky.product-setup/1` envelope from ky-primitives/applysetup. The CLI talks to
`<KYPULSE_DATA_DIR>/installer/setup.sock`; the active server owns this private
socket, so inspection never opens/migrates a separate database or generates keys.
The shared package must be released before a standalone product release can consume it.

In the Compose install the binary is `/app/kypulse` and runs as uid 10001, so the bundle and
every file it names must be readable by that uid inside the container (a mounted path):

```sh
docker compose exec app /app/kypulse apply-setup --file /app/data/<bundle>.json
```

`--check` verifies a bundle that was already applied: any absent configuration, identity,
monitoring target or source is an error. It is not a dry run; run it after apply.

The SSO section names HTTPS `issuerUrl`, `clientId`, `clientSecretFile`, the exact
`redirectUri` (`KYPULSE_APP_URL/api/sso/kysignon/callback`) and `directorySecretFile`.
Every credential is an absolute private runtime-owned regular file on the target.
Generic OIDC/SAML or differing existing KyIdentity settings are refused. A new
installer configuration disables the default auto-provisioning; explicit existing
configuration is preserved and conflicts require operator resolution.

`admins` and `everyday` contain exact issuer/subject/username/email bindings.
Only absent identities are created, with administrator and viewer roles respectively,
without local passwords. Existing names/subjects cannot be adopted. Configuration,
accounts and ownership commit together with one keyed audit-chain event. Inactive or
role-drifted identities require explicit resolution; rerun never reactivates them.

**Offboarding does not reach kyPulse from KyIdentity today.** `directorySecretFile` keys the
legacy KySignOn webhook at `POST /api/sso/kysignon/sync`, which accepts only
`hex(HMAC-SHA256(body))` in `X-KySignOn-Signature` over a JSON body with `event`, `id`,
`status`, `role` and Unix `timestamp`; `user.deactivated` and role/status changes revoke that
user's sessions. KyIdentity's `suiteWebhook` connector signs with ky-primitives `syncauth` v1
(`v1=`-prefixed MAC over a canonical string, SCIM body), so kyPulse answers 401
`invalid webhook signature` and the connector stays pending. Disabling a user in KyIdentity
can only stop new SSO sign-ins; existing kyPulse sessions last until they expire, up to 7
days after sign-in, and kyPulse has no user-management API to end them sooner.

The application is a monitoring/control product; administrator access does not grant
ordinary mail, notes, drive or vault content access in other products.

The backup section currently supports directory, retention and default interval.
A recovery pairing section is refused before writes; use the protected product backup
interface until its installer path is implemented. KyYard enrollment,
suite identity-client/group composition and independent restore
acceptance remain open gates.

The product-specific `monitoring` extension creates explicitly owned targets and sources:

```json
"monitoring": {
  "targets": [{"key":"identity","name":"KyIdentity","url":"https://identity.example/healthz","intervalSec":30}],
  "sources": [{"key":"offboard","name":"offboard","target":"identity","tokenFile":"/data/installer/offboard-source.token","offboardingAlerts":true}]
}
```

Use at most 32 targets and 32 sources, with stable keys and distinct names. A source's
`target` refers to a target key in this bundle. URLs use the native health egress policy;
LAN destinations and HTTP are supported, while loopback/link-local and the product's own
configured origin are refused. The source token must already exist in a protected
runtime-owned file as 32 bytes encoded in lowercase hex; generate it once on its target
and retain it through resume. SSO/source credentials must differ. Only its hash enters the
native source table; reports and ownership definitions contain no token. Pairing-code
claim is unnecessary for this explicitly authorized container-local setup path.

Targets/sources, accounts, ownership and one keyed setup audit commit together. Foreign
names or token hashes, different installation ownership, edited definitions, missing/drifted
owned targets or revoked sources are refused without adoption, regrant or data deletion.
Omitting an installed object preserves it. Reports include `configuration` objects named
`monitoring.target.<key>` and `monitoring.source.<key>` with their native IDs. Check is
read-only; resume preserves poll/silence and ingestion receipt state. Monitoring rows are
live immediately and require no restart. An `offboardingAlerts:true` source advertises the
native alert capability and produces failures through the existing ingestion/monitor path.
Workstation-side guided token distribution and full suite composition remain release gates.
Active installer source tokens now travel inside the sealed SQLite capsule's private
`config/installer-source-credentials.json` member. Collection verifies protected file contents
against the same database snapshot; missing or changed tokens refuse backup. Restore drills
check all active source bindings and token hashes. Revoked sources are not reactivated and
require no token file. The member records original target paths for explicit restoration;
extraction stays inside the private restore directory. Placement back on product and bridge
targets, including bridge receipt state, remains an end-to-end suite restore gate.

Applying stages immutable configuration and reports `activation: restart` until observed
live. `apply-setup --restart` requests graceful shutdown; the workload controller must
restart the product. KyQuickStart waits for a present-only live check before success.
Secrets remain in target files, including through restart and resume. Public
`GET /api/installer/network` reports only `clientIp`, `proxyHeadersTrusted` and `scheme`
for the edge verifier; cached suite `/healthz` remains unchanged.

Verify with `go test -race ./internal/store ./internal/config ./internal/api ./internal/auth ./cmd/server`
in the explicit development Go workspace. Tests cover ownership/conflict rollback, the
keyed audit chain, atomic monitoring ownership/source enrollment, actual private IPC activation, real administrator/viewer authorization
and legacy-format signed directory revocation. Source implementation is not a published or deployed suite.
