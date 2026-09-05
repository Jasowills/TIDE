# OWASP Top 10 (2025) coverage — TIDE

Every category maps to at least one gating test/scan, or a recorded accepted
risk with justification. No category is silently absent. Normative gates live
in `.github/workflows/security.yml`; runnable proofs live in Go tests (a Go +
React codebase — httptest assertions are the deterministic equivalent of the
skill's Playwright patterns; no browser suite exists to host a
`--project=security`).

| ID | Category | Status | Proof (gate) |
|---|---|---|---|
| A01 | Broken Access Control | **Covered** | Tenant isolation enforced at query layer (`TestTenantIsolation`, api). Webhook SSRF guard: scheme allow, DNS-resolve + private/metadata block, redirect re-validation, connect-time dial guard (`TestValidateURLBlocksSSRF`, `TestDispatchBlockedEmitsZeroRequests`, `TestRedirectToPrivateRefused`, webhooks). Firing proof paired with `TestDispatchAllowedReachesServer` so the block tests are not vacuous. |
| A02 | Security Misconfiguration | **Covered** | TRACE/TRACK/CONNECT rejected 405 (`TestTraceDisabled`, api). No stack traces in errors — all 500s generic (`TestErrorContractLeaksNothing`). No default creds in code; compose secrets documented in ops. |
| A03 | Supply Chain | **Covered** | OSV-Scanner gate, split per lockfile (Go + npm steps in `security.yml`, each failing non-zero). Split because osv-scanner v2.5.1 honors `IgnoredVulns` only in single-source scans; single-line `scan-args` because the action does not split embedded newlines. Recorded ignores in `osv-scanner.toml` (openpgp deprecation + two dev-only LHCI findings, each with justification + re-check trigger). `package-lock.json` committed; console Dockerfile installs via `npm ci`. Dependabot (`.github/dependabot.yml`). Semgrep supply-chain rules in SAST gate. |
| A04 | Cryptographic Failures | **Covered** | Baseline headers on every response: nosniff, DENY framing, no-referrer, restrictive CSP (`TestSecurityHeaders`, api). Webhook HMAC-SHA256 + 5-min replay window, verified both directions (`TestSignVerifyRoundTrip`, webhooks; consumer example in `examples/`). Secrets never logged (audited; DSN/secret grep clean). TLS termination is deployment concern — compose is local-dev plain HTTP, documented. |
| A05 | Injection | **Covered** | Injection-shaped input validates-or-4xx, never 500s, no DB vocabulary in errors (`TestInjectionInputFailsClosed`, api). Parameterized PG queries only (`$1…`, store) — no string-built SQL. Semgrep `p/owasp-top-ten` gates regressions. |
| A06 | Insecure Design | **Covered** | Ingest rate-limited per IP with 429 + Retry-After; burst test requires throttling (`TestIngestBurstThrottled`, api). Webhook valves: cooldown, max-actions/hour, DLQ (rules/webhooks tests). URL-accepting features (webhooks) are allow-architecture: public-http(s)-only (A01). |
| A07 | Authentication Failures | **Accepted risk (V1)** | V1 ships NO authentication by spec design (§2.11: API keys/service tokens only, OAuth2/OIDC deferred; local-dev bind). There is no login, session, JWT, or RBAC to negative-path test — adding auth tests against nonexistent auth would be theater. The committed control: tenant-scoped queries + unscoped-query rejection (A01). Auth arrives with API keys; this row flips to Covered with expiry/replay/role-escalation tests at that point. |
| A08 | Integrity Failures | **Covered** | CSP present, no `unsafe-inline`/`unsafe-eval` (`TestSecurityHeaders`). Console has no CDN scripts (vendored `node_modules`, no SRI surface). Deserialization: strict JSON decode with 4xx on malformed (ingest tests). |
| A09 | Logging & Alerting | **Partially covered** | Rule triggers audit-logged per evaluation (`rules` trace + `/v1/rules/triggers`); webhook deliveries + dead-letters recorded (webhooks). Logs carry no secrets/PII (audited). **Gap (recorded)**: no alerting pipeline on thresholds (failed logins N/A — no auth; DLQ growth unalerted). Alerting ships with production API-key auth. |
| A10 | Exceptional Conditions | **Covered** | Generic `internal error` 500s; bus outage → **503 retryable** with computed events persisted to PG (visible, never stranded); backend failures fail closed (`TestErrorContractLeaksNothing`, `TestBusFailureIs503`). PG-down degrades to bounded memory buffer, never fails open (`TestPostgresDownDoesNotKillIngestion`, boot). Chaos suite: malformed/duplicate/late storms + live Redis restart rebuild (tests). Fuzzing beyond fixed fixtures: deferred, ZAP active scan covers breadth in CI. |
| LLM01–10 | LLM Top 10 | **Not applicable** | TIDE ships no LLM features (no chatbot/RAG/agent/copilot). If one is added, `ai-system-testing` owns LLM01/02 deep coverage; LLM05 re-runs A05 checks on model output. |

## Verification record

- Firing proofs: SSRF block test asserts **zero server hits** alongside an allow-path test asserting **exactly one hit** — the negative assertions provably reach the code.
- Matcher form: no `toBeOneOf` anywhere (`grep -r toBeOneOf` clean — Go tables + explicit sets used throughout).
- Gate check (empirical, 2026-09-04): OSV-Scanner exited **1** on the pre-fix tree (x/net, x/sys, x/text, x/crypto, otel/sdk, klauspost/compress, vite×4, esbuild) — all upgraded to fixed versions; post-fix scan reports **0 affected packages**, with the single remaining advisory (GO-2026-5932, openpgp deprecation, no fix published, package absent from our build graph) recorded in `osv-scanner.toml` with justification. The gate demonstrably fires on findings and clears without them.
- SAST check (empirical, 2026-09-04): Semgrep `p/owasp-top-ten` (305 rules, 108 targets) reports **0 findings** after SHA-pinning actions, 7-day Dependabot cooldown, non-root console user, crypto-rand jitter, and two justified `nosemgrep` sites (seeded sim determinism is a spec requirement).
- Secrets: `grep -riE 'AKIA|ghp_|BEGIN .* PRIVATE KEY'` clean; TruffleHog `--only-verified` gates every PR.
