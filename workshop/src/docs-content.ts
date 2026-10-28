export type DocSection = {
  id: string;
  title: string;
  group: string;
  summary: string;
  body: string;
};

/** In-app Pathfinder documentation suite (HTML fragments; already trusted). */
export const DOC_SECTIONS: DocSection[] = [
  {
    id: 'overview',
    title: 'What Pathfinder is',
    group: 'Start here',
    summary: 'Deterministic ADR compliance: extract → review → bind → evidence → assess.',
    body: `
<p>Pathfinder turns accepted Architecture Decision Records (ADRs) and plain-English technical specifications into <strong>reviewable obligations</strong>, binds them to typed verifiers, collects analyzer evidence, and emits Markdown / JSON / SARIF assessments — with <strong>zero runtime LLM calls</strong> on the assess path.</p>
<ul>
  <li><strong>GitHub Issues</strong> carry fenced snippets + tech specs; ADR Assessment sandboxes fences and assesses against the OWASP/AWS corpus. Inputs are tagged <code>submission_kind</code>: <code>tech_spec</code> (ADR/prose) vs <code>code_snippet</code> (source/IaC) so parsers and KPIs can trend separately.</li>
  <li><strong>Repository CI</strong> uses the live catalog (<code>adr/requirements.json</code>) plus OpenGrep / Checkov evidence for pull requests.</li>
  <li><strong>This workshop</strong> is a GitOps command center: Actions emit snapshots, <code>workshop-data</code> stores history, Pages serves the SPA.</li>
</ul>
<p>Guiding rule: a clean scan is never “the whole estate is compliant.” Coverage and status are explicit per obligation.</p>
<p>Related: <a href="#/docs/architecture">Architecture</a> · <a href="#/docs/lifecycle">Spec → enforcement</a> · <a href="#/docs/integrations">Integrations</a> · <a href="#/sample">Sample journey</a></p>`,
  },
  {
    id: 'architecture',
    title: 'Architecture',
    group: 'Start here',
    summary: 'Assessment engine, analyzers, evidence contract, and what stays out of runtime.',
    body: `
<p>Pathfinder is an <strong>orchestration and evidence layer</strong> over typed verifiers and pinned analyzers. The Go assessment engine in <code>internal/assessment</code> remains the foundation; scale comes from reusable patterns across many ADRs, not one custom verifier per decision.</p>
<h3>Runtime flow</h3>
<ol class="docs-steps">
  <li>Issue Form or repository CI supplies context; submitted issue code is never executed.</li>
  <li>Catalog validation selects accepted requirements (<code>adr/requirements.json</code> for repo CI; corpus catalog for issue surface).</li>
  <li>Typed verifier / analyzer adapters evaluate in-scope evidence (Go AST, JSON, OpenGrep, Checkov).</li>
  <li>Engine emits per-requirement results, overall verdict, coverage (<code>COMPLETE</code> / <code>PARTIAL</code> / <code>NONE</code>), Markdown, JSON, SARIF.</li>
  <li>Workflows comment reports, upload artifacts, and optionally <code>workshop-emit</code> for Pages.</li>
</ol>
<h3>Three-layer model</h3>
<ul>
  <li><strong>Source ADR</strong> — immutable Markdown, digest, sections, metadata.</li>
  <li><strong>Obligation</strong> — cited candidate or approved interpretation (modality, applicability, enforcement mode).</li>
  <li><strong>Verification pattern</strong> — reusable analyzer/policy contract shared by many obligations.</li>
</ul>
<p>Optional LLM assist is <strong>batch-time only</strong>. Runtime assessment stays deterministic. See repo <code>docs/architecture.md</code> and <code>docs/scaling-adr-design.md</code>.</p>`,
  },
  {
    id: 'lifecycle',
    title: 'Specification → enforcement lifecycle',
    group: 'Start here',
    summary: 'Eight stages from ingest to honest coverage reporting.',
    body: `
<ol class="docs-steps">
  <li><strong>Ingest</strong> — preserve source bytes, identity, version, and locations (<code>ingest-adrs</code>).</li>
  <li><strong>Extract</strong> — candidate obligations with modality, citations, ambiguity (<code>extract-obligations</code>). Never executable by default.</li>
  <li><strong>Confidence</strong> — heuristic score → calibrated <code>confidence_p</code> → conformal high/medium/low; low escalate to batch assist. Details: <a href="#/docs/confidence">Confidence scoring</a>.</li>
  <li><strong>Assist (batch only)</strong> — optional LLM proposals; never auto-approved (<code>assist-obligations</code>).</li>
  <li><strong>Human review</strong> — approve / reject / bind in the obligation registry.</li>
  <li><strong>Bind</strong> — map to a verifier pattern (Go AST, JSON, OpenGrep, Checkov, manual-review).</li>
  <li><strong>Evidence</strong> — pinned analyzers, SARIF normalization, digests for binary/config/input/raw output.</li>
  <li><strong>Assess</strong> — per-requirement <code>PASS</code> / <code>FAIL</code> / <code>INDETERMINATE</code> / <code>NOT_APPLICABLE</code> / <code>REVIEW_REQUIRED</code> / <code>EVIDENCE_MISSING</code>.</li>
</ol>
<p>Candidate extraction alone is never approval. See <code>docs/specification-to-enforcement.md</code>.</p>`,
  },
  {
    id: 'issue-flow',
    title: 'GitHub Issue assessment flow',
    group: 'Pipelines',
    summary: 'How [ADR Assessment] issues become reports and workshop snapshots.',
    body: `
<ol class="docs-steps">
  <li>Open an issue titled <code>[ADR Assessment] …</code> with tech-spec prose and/or language-tagged fences.</li>
  <li><code>issue_sandbox_extract.py</code> materializes fences into a temp tree (code is never executed). Fence-only bodies classify as <code>submission_kind=code_snippet</code>; ADR-like prose as <code>tech_spec</code>; mixed stays unspecified unless <code>--submission-kind</code> is set.</li>
  <li>Pinned <strong>OpenGrep</strong> and <strong>Checkov</strong> scan the sandbox; evidence JSON + raw SARIF are written.</li>
  <li><code>adrctl assess-repo --issue-surface</code> evaluates <code>testdata/corpus/requirements.json</code> (200 ADRs). Manual-review and missing analyzers become <code>NOT_APPLICABLE</code> so they do not force indeterminate noise.</li>
  <li>Report comments on the issue; artifacts upload; Workshop Publish merges a snapshot for Pages.</li>
</ol>
<p>Smoke titles like <code>[manual-review-smoke]</code> keep a small catalog for regression of issue-surface semantics.</p>`,
  },
  {
    id: 'repo-ci',
    title: 'Repository CI & PR gates',
    group: 'Pipelines',
    summary: 'Live catalog, OpenGrep fixtures, Checkov IaC proof, reviewdog.',
    body: `
<ul>
  <li><code>adr/requirements.json</code> is the accepted example catalog for repo CI (not the 200-ADR corpus).</li>
  <li>OpenGrep is pinned (version + binary SHA-256); vulnerable/clean fixtures under <code>testdata/analyzer-fixtures</code> prove rule families.</li>
  <li>Checkov is pinned for Terraform/CloudFormation/Kubernetes/Dockerfile; IaC fixtures live under <code>…/iac/</code>.</li>
  <li>reviewdog annotates SARIF on same-repo PRs; fork constraints still apply.</li>
  <li>Full repository assess keeps <code>IssueSurface=false</code>: manual-review → <code>REVIEW_REQUIRED</code>, missing analyzer → <code>EVIDENCE_MISSING</code> (fail closed).</li>
</ul>
<p>CI policy: <code>FAIL</code>, <code>EVIDENCE_MISSING</code>, <code>ERROR</code>, and overall <code>INDETERMINATE</code> are merge-blocking. See <code>docs/architecture.md</code>.</p>`,
  },
  {
    id: 'pr-scan',
    title: 'PR URL scan (reviewdog + Security)',
    group: 'Pipelines',
    summary: 'Paste any GitHub PR URL; Pathfinder posts sparse reviewdog feedback and optional Security-tab SARIF.',
    body: `
<ol class="docs-steps">
  <li>Open <strong>Pathfinder PR scan</strong> issue template; paste a PR URL (this repo or another).</li>
  <li>Select reviewdog reporters and/or <strong>Upload SARIF to GitHub code scanning</strong>.</li>
  <li>Or skip the issue: <strong>Actions → Pathfinder PR Scan → Run workflow</strong> with <code>pr_url</code> + reporter flags.</li>
  <li>Workflow shallow-fetches the PR head as data only (never runs untrusted PR code), assesses with pinned OpenGrep/Checkov + <code>assess-repo --issue-surface</code>.</li>
  <li>Shared finding model → rdjson (reviewdog) and SARIF 2.1.0 (Security). Bodies are short: severity, ADR, why, evidence, next/docs.</li>
  <li>N/A floods are omitted; inline comments capped; one PR summary with counts. Stable check name: <code>pathfinder-pr-scan</code> (default <code>fail_level=any</code>: <strong>failure</strong> when deviations exist, <strong>success</strong> when clean — require in branch protection).</li>
</ol>
<p><strong>Auth:</strong> same-repo uses <code>GITHUB_TOKEN</code>. Cross-repo: prefer the <a href="#/docs/github-app">GitHub App</a> (<code>APP_ID</code> + <code>APP_PRIVATE_KEY</code>) or legacy PAT <code>PATHFINDER_REVIEW_TOKEN</code> / optional <code>PATHFINDER_SARIF_TOKEN</code>. Full operator doc: <code>docs/pr-scan.md</code> · setup map: <a href="#/docs/integrations">Integrations</a>.</p>`,
  },
  {
    id: 'github-app',
    title: 'GitHub App setup (PR scan)',
    group: 'Integrations',
    summary: 'Create the Pathfinder App, paste secrets, install on repos, mint short-lived tokens for reviewdog + Security.',
    body: `
<p>Actions-native MVP is shipped: the App supplies ≤1h installation tokens via <code>actions/create-github-app-token</code>. The reusable workflow remains the scan engine. A custom webhook server is out of scope (manifest webhook stays inactive).</p>
<h3>1. Create the App in GitHub</h3>
<ol class="docs-steps">
  <li>Owner account → <strong>Settings → Developer settings → GitHub Apps → New GitHub App</strong> (org: <code>…/organizations/&lt;ORG&gt;/settings/apps/new</code>).</li>
  <li><strong>Name:</strong> Pathfinder PR Scan (must be unique). <strong>Homepage:</strong> this repository URL.</li>
  <li><strong>Webhook:</strong> uncheck Active (Actions-native; no server).</li>
  <li><strong>Repository permissions:</strong> Checks write · Contents read · Pull requests write · Security events write · Metadata read. Do <em>not</em> grant workflows, administration, or contents write.</li>
  <li>Optional events (future webhook): Pull request, Installation, Installation repositories. Manifest: <code>.github/pathfinder-app-manifest.json</code>.</li>
  <li><strong>Create GitHub App</strong> → <strong>Private keys → Generate</strong> → save the <code>.pem</code>. Copy <strong>App ID</strong> (and optional <strong>Client ID</strong>).</li>
</ol>
<h3>2. Install on repositories</h3>
<ol class="docs-steps">
  <li>App settings → <strong>Install App</strong> → choose account → <strong>Only select repositories</strong> → pick <code>pf_2</code> and each consumer repo → Install.</li>
  <li>Confirm the install shows the permissions above.</li>
</ol>
<h3>3. Secrets (host or consumer Actions)</h3>
<table class="docs-table">
  <thead><tr><th>Secret</th><th>Required</th><th>Value</th></tr></thead>
  <tbody>
    <tr><td><code>APP_ID</code></td><td>Yes*</td><td>Numeric App ID from App settings</td></tr>
    <tr><td><code>APP_PRIVATE_KEY</code></td><td>Yes</td><td>Full PEM including BEGIN/END lines (escaped <code>\\n</code> OK)</td></tr>
    <tr><td><code>APP_CLIENT_ID</code></td><td>Optional</td><td>When set, host workflow prefers client-id mint over App ID</td></tr>
  </tbody>
</table>
<p>*Provide <code>APP_ID</code> and/or <code>APP_CLIENT_ID</code> plus <code>APP_PRIVATE_KEY</code>. Until secrets exist, <strong>Pathfinder App PR Scan</strong> skips with a notice (does not fail every PR).</p>
<p><strong>Legacy PAT path</strong> (still supported): <code>PATHFINDER_REVIEW_TOKEN</code> with <code>contents:read</code>, <code>pull_requests:write</code>, <code>checks:write</code> on the target; optional <code>PATHFINDER_SARIF_TOKEN</code> for Security-only. Prefer App tokens for multi-repo installs.</p>
<h3>4. Host workflow</h3>
<ol class="docs-steps">
  <li><code>.github/workflows/pathfinder-app-pr-scan.yml</code> runs on <code>pull_request</code> (opened/synchronize/reopened) and <code>workflow_dispatch</code>.</li>
  <li>Mints an installation token scoped to the current repo with least-privilege permission inputs, then calls the reusable scan with that token as <code>review_token</code> / <code>sarif_token</code>.</li>
  <li>Same-repo E2E without App secrets: use <strong>Actions → Pathfinder PR Scan → Run workflow</strong> (<code>GITHUB_TOKEN</code>) — see <a href="#/docs/pr-scan">PR URL scan</a>.</li>
</ol>
<p>Repo design doc: <code>docs/github-app-design.md</code> · consumer example: <code>examples/consumer-pr-scan-app.yml</code> · map of all surfaces: <a href="#/docs/integrations">Integrations</a>.</p>`,
  },
  {
    id: 'integrations',
    title: 'Integrations map',
    group: 'Integrations',
    summary: 'Host/consumer workflows, reviewdog + SARIF, issue templates, and workshop Pages as the analytics surface.',
    body: `
<p>One assessment pipeline, several ways to trigger and authenticate it. Prefer the <a href="#/docs/github-app">GitHub App</a> for cross-repo tokens; keep the issue / dispatch host workflow for same-repo ops.</p>
<h3>Workflows</h3>
<table class="docs-table">
  <thead><tr><th>Workflow</th><th>Trigger</th><th>Auth</th><th>Role</th></tr></thead>
  <tbody>
    <tr><td><code>pathfinder-pr-scan.yml</code></td><td>Issue template + <code>workflow_dispatch</code></td><td><code>GITHUB_TOKEN</code> or <code>PATHFINDER_REVIEW_TOKEN</code></td><td>Host operator scan (issue authors gated OWNER/MEMBER/COLLABORATOR)</td></tr>
    <tr><td><code>pathfinder-app-pr-scan.yml</code></td><td><code>pull_request</code> + dispatch</td><td><code>APP_ID</code> / <code>APP_CLIENT_ID</code> + <code>APP_PRIVATE_KEY</code></td><td>Mint install token → reusable scan</td></tr>
    <tr><td><code>pathfinder-pr-scan-reusable.yml</code></td><td><code>workflow_call</code></td><td>Caller-supplied <code>review_token</code> / <code>sarif_token</code></td><td>Shared engine: fetch head as data → OpenGrep/Checkov → assess → reviewdog → optional SARIF</td></tr>
    <tr><td><code>adr-assessment.yml</code></td><td><code>[ADR Assessment]</code> issues</td><td>repo token</td><td>Sandbox fences vs 200-ADR corpus; feeds workshop snapshots</td></tr>
    <tr><td><code>workshop-publish.yml</code></td><td><code>workflow_run</code> / dispatch</td><td>Pages permissions</td><td>Merge <code>workshop-data</code> + deploy this SPA</td></tr>
  </tbody>
</table>
<h3>Consumer copy-paste</h3>
<ol class="docs-steps">
  <li><strong>App (preferred):</strong> copy <code>examples/consumer-pr-scan-app.yml</code> → mint token with <code>create-github-app-token</code> → <code>uses: tyler-beach/pf_2/.github/workflows/pathfinder-pr-scan-reusable.yml@main</code>.</li>
  <li><strong>PAT (legacy):</strong> copy <code>examples/consumer-pr-scan.yml</code> → secret <code>PATHFINDER_REVIEW_TOKEN</code> (optional <code>PATHFINDER_SARIF_TOKEN</code>).</li>
  <li>Pin <code>pathfinder_ref</code> to a tag/SHA when ready. Inputs: <code>reporters</code>, <code>upload_sarif</code>, <code>filter_mode</code>, <code>catalog</code>, <code>fail_level</code> (default <code>any</code>).</li>
</ol>
<h3>reviewdog reporters + Security SARIF</h3>
<ul>
  <li><code>github-pr-review</code> — inline review comments (default with check)</li>
  <li><code>github-pr-check</code> / <code>github-check</code> — Checks API / commit Check Run (<strong>require <code>pathfinder-pr-scan</code></strong> in branch protection)</li>
  <li><code>github-pr-annotations</code> — job annotations · <code>local</code> — artifact only</li>
  <li><strong>Upload SARIF</strong> / <code>upload_sarif=true</code> → Security tab via code-scanning API (<code>pathfinder/pr-scan</code> automation id). Target needs code scanning available (GHAS on private).</li>
</ul>
<p>Filter modes: <code>added</code> (default), <code>diff_context</code>, <code>file</code>, <code>nofilter</code>. Findings = deviations only (never N/A/PASS). Default <code>fail_level=any</code> (+ reviewdog <code>-level=error</code>) makes <code>pathfinder-pr-scan</code> <strong>fail</strong> when deviations exist and <strong>succeed</strong> when clean; <code>none</code> = report-only / neutral. Check is posted on the PR head SHA.</p>
<h3>Issue templates</h3>
<ul>
  <li><strong>Pathfinder PR scan</strong> — paste PR URL; checkboxes for reporters + Security; title prefix <code>[Pathfinder PR Scan]</code> or label <code>pathfinder-pr-scan</code>.</li>
  <li><strong>ADR compliance assessment</strong> — <code>[ADR Assessment]</code> title; tech-spec prose and/or language-tagged fences; sandbox + corpus assess; successful runs publish workshop snapshots.</li>
</ul>
<h3>Workshop Pages (analytics surface)</h3>
<ol class="docs-steps">
  <li>Settings → Pages → Source: <strong>GitHub Actions</strong> (once). Environment <code>github-pages</code>.</li>
  <li>CI / ADR Assessment / Corpus Eval / LLM Assist emit <code>workshop-inputs*</code>; <strong>Workshop Publish</strong> merges history on <code>workshop-data</code> and deploys the SPA.</li>
  <li>Manual refresh: <strong>Actions → Workshop Publish → Run workflow</strong> (optional <code>source_run_id</code> or fixture snapshot).</li>
  <li>PR-scan dig-in: <a href="#/runs"><code>#/runs?run_id=…</code></a>. Docs live under <code>#/docs/…</code> (this suite). Site never holds secrets.</li>
</ol>
<p>Deep dives: <a href="#/docs/github-app">GitHub App</a> · <a href="#/docs/pr-scan">PR URL scan</a> · <a href="#/docs/issue-flow">Issue assessment</a> · <a href="#/docs/workshop-data">Workshop data plane</a> · repo <code>docs/pr-scan.md</code> · <code>docs/github-app-design.md</code> · <code>docs/workshop.md</code>.</p>`,
  },
  {
    id: 'workshop-data',
    title: 'Workshop data plane',
    group: 'Pipelines',
    summary: 'workshop-emit → workshop-data branch → Pages.',
    body: `
<ol class="docs-steps">
  <li>CI / ADR Assessment / Corpus Eval / LLM Assist call <code>adrctl workshop-emit</code> and upload <code>workshop-inputs*</code>.</li>
  <li><strong>Workshop Publish</strong> appends <code>metrics/runs.jsonl</code>, stores <code>snapshots/&lt;run_id&gt;/</code>, updates <code>latest.json</code> + <code>manifest.json</code> on <code>workshop-data</code>.</li>
  <li>Pages serves the SPA shell (content-hashed) and short-cache data files with cache-busting queries.</li>
</ol>
<p>Humans write back via Issues / PR templates / <code>workflow_dispatch</code>. The static site never holds secrets. Local: <code>cd workshop &amp;&amp; npm ci &amp;&amp; npm run dev</code>. Setup map: <a href="#/docs/integrations">Integrations</a>.</p>`,
  },
  {
    id: 'verifiers',
    title: 'Verifier types',
    group: 'Assessment engine',
    summary: 'What can prove an obligation today.',
    body: `
<table class="docs-table">
  <thead><tr><th>Type</th><th>Input</th><th>Notes</th></tr></thead>
  <tbody>
    <tr><td><code>go-import-boundary</code></td><td>Go AST</td><td>Forbidden import prefixes under path scope.</td></tr>
    <tr><td><code>json-required-keys</code></td><td>JSON file</td><td>Top-level keys; missing → findings.</td></tr>
    <tr><td><code>json-number-range</code></td><td>JSON file</td><td>Inclusive numeric bounds.</td></tr>
    <tr><td><code>analyzer-rule</code></td><td>OpenGrep / Checkov ScanResult</td><td>Exact <code>rule_ids</code>, path/extension scope, complete digests.</td></tr>
    <tr><td><code>manual-review</code></td><td>Human</td><td>Repo: <code>REVIEW_REQUIRED</code>. Issue surface: <code>NOT_APPLICABLE</code>.</td></tr>
  </tbody>
</table>
<p>Unknown verifier types are <code>UNSUPPORTED</code>, never silently treated as review-required. Incomplete analyzer evidence never passes.</p>`,
  },
  {
    id: 'statuses',
    title: 'Statuses & verdicts',
    group: 'Assessment engine',
    summary: 'How requirement outcomes roll up.',
    body: `
<table class="docs-table">
  <thead><tr><th>Status</th><th>Meaning</th></tr></thead>
  <tbody>
    <tr><td><code>PASS</code></td><td>Applicable check succeeded with evidence.</td></tr>
    <tr><td><code>FAIL</code></td><td>Confirmed violation; wins the overall verdict.</td></tr>
    <tr><td><code>NOT_APPLICABLE</code></td><td>Scope empty, issue-surface N/A, or analyzer not collected for issue.</td></tr>
    <tr><td><code>REVIEW_REQUIRED</code></td><td>Human judgment still required (repo).</td></tr>
    <tr><td><code>EVIDENCE_MISSING</code></td><td>Required analyzer/inventory incomplete (fail closed).</td></tr>
    <tr><td><code>INDETERMINATE</code> / <code>ERROR</code> / <code>UNSUPPORTED</code></td><td>Cannot establish compliance.</td></tr>
  </tbody>
</table>
<p>Overall <code>PASS</code> requires at least one applicable <code>PASS</code> and no failures / unresolved statuses. All-<code>NOT_APPLICABLE</code> is still indeterminate (nothing proved). Filter live results on <a href="#/assess">Assess</a>.</p>`,
  },
  {
    id: 'opengrep',
    title: 'OpenGrep adapter',
    group: 'Analyzers',
    summary: 'Pinned multi-language structural scanner.',
    body: `
<ul>
  <li>CLI: <code>adrctl scan-opengrep</code> → plan manifest → normalized JSON + raw SARIF.</li>
  <li>Rules: <code>rules/pathfinder-security.yml</code> (merged packs: core, OWASP, AWS SDK, misc promotions).</li>
  <li>Evidence verification re-parses raw SARIF and checks binary/config/input digests.</li>
  <li>Corpus patterns for injection, crypto, secrets, SSRF, logging, hardening, supply-chain Git deps, etc. bind here.</li>
  <li>Adapter: version pin, timeout/cancellation, restricted env, symlink-safe repo-local config, stable fingerprints.</li>
</ul>
<p>Language support is claimed <strong>per rule family</strong> with vulnerable/clean fixtures — not blanket semantic support. See <code>docs/language-support.md</code>.</p>`,
  },
  {
    id: 'checkov',
    title: 'Checkov adapter',
    group: 'Analyzers',
    summary: 'True IaC assessment for AWS corpus families.',
    body: `
<ul>
  <li>CLI: <code>adrctl scan-checkov</code>; config <code>rules/checkov.yml</code> (terraform, cloudformation, kubernetes, dockerfile).</li>
  <li>Findings normalize to bare <code>CKV_*</code> / <code>CKV2_*</code> rule IDs for catalog matching.</li>
  <li>Corpus binds SG/network/IAM/encryption/logging/backup/compute/infra.policy patterns to Checkov.</li>
  <li>On issue/sandbox, Checkov runs alongside OpenGrep; if no IaC scope matches, those requirements are <code>NOT_APPLICABLE</code>.</li>
</ul>`,
  },
  {
    id: 'plan-executor',
    title: 'Plan / registry / executor',
    group: 'Analyzers',
    summary: 'Deterministic analyzer plans, local execute-plan, obligation review registry.',
    body: `
<h3>Analyzer plan manifests</h3>
<p>A plan is a reproducible description of analyzer work for a repository snapshot, separate from execution output. Identical inputs produce byte-identical manifest JSON. Keys bind source-tree digest, inventory, config SHA-256, analyzer version, scope, and execution policy (timeouts, retries, output caps).</p>
<ul>
  <li><code>adrctl plan-repo</code> — build manifest</li>
  <li><code>adrctl verify-plan</code> — recheck against current tree</li>
  <li><code>adrctl execute-plan</code> — local runner; cache outside the repo; fail closed on stale/corrupt evidence</li>
</ul>
<p><code>internal/plan</code> owns planning/execution; <code>scan-opengrep</code> uses the same contracts. Production runner supports pinned OpenGrep; other analyzers fail closed until registered. Cloud scheduling is not claimed implemented. Details: <code>docs/analyzer-plan-manifest.md</code>.</p>
<h3>Obligation review registry</h3>
<ul>
  <li><code>review-init</code> creates <code>adr/obligation-reviews.json</code> (never overwrites history).</li>
  <li><code>record-obligation-review</code> records approve/reject/bind decisions.</li>
  <li><code>validate-obligation-reviews</code> / <code>assess-obligations</code> enforce bindings; <code>--enforce-blocking</code> fails closed on pending or non-PASS blocking items.</li>
  <li>Stale source digests or changed verifier definitions invalidate approvals until re-review.</li>
</ul>
<p>Extraction never mutates the accepted catalog. See <code>docs/obligation-review.md</code>.</p>`,
  },
  {
    id: 'corpus',
    title: 'OWASP + AWS corpus',
    group: 'Corpus & patterns',
    summary: '200 MADRs with RFC 2119 Decision language.',
    body: `
<ul>
  <li>ADRs <code>ADR-3001…3120</code> (OWASP) and <code>ADR-3201…3280</code> (AWS) under <code>testdata/corpus/adrs/</code>.</li>
  <li>Labels in <code>labels.json</code>; bindings generated by <code>scripts/generate_corpus_bindings.py</code>.</li>
  <li>Many ADRs → few reusable patterns in <code>adr/patterns.json</code> → shared analyzers (not 1:1 Semgrep clones).</li>
  <li>Extract harness + Promptfoo evaluate candidate quality; scores are <strong>not</strong> compliance verdicts.</li>
</ul>
<p>Browse labels in this UI: <a href="#/corpus">Corpus</a>. Label via PR with <code>[corpus-label]</code>.</p>`,
  },
  {
    id: 'patterns',
    title: 'Pattern catalog',
    group: 'Corpus & patterns',
    summary: 'Reusable verifier families.',
    body: `
<p>Patterns declare allowed verifier types, inputs, applicability, and blind spots. Examples:</p>
<ul>
  <li><code>security.injection</code>, <code>security.crypto</code>, <code>security.secrets</code>, <code>security.ssrf</code></li>
  <li><code>security.aws_*</code> / <code>configuration.aws_*</code> — SDK misuse via OpenGrep or IaC via Checkov</li>
  <li><code>infra.policy</code> — Checkov aggregate policy surface</li>
  <li><code>manual_review</code> — subjective / business-semantic leftovers</li>
</ul>
<p>Joern / CPG remains deferred until ≥10 accepted ADRs need cross-file data-flow evidence OpenGrep cannot supply.</p>`,
  },
  {
    id: 'obligations',
    title: 'Obligation review registry',
    group: 'Governance',
    summary: 'Candidates never mutate the accepted catalog automatically.',
    body: `
<ul>
  <li>Extraction produces review-required candidates with stable IDs and source citations.</li>
  <li>Reviewers approve blocking / advisory / review_required bindings via <code>record-obligation-review</code>.</li>
  <li>Stale source digests or changed verifier definitions invalidate approvals until re-review.</li>
  <li><code>assess-obligations --enforce-blocking</code> fails closed on pending or non-PASS blocking bindings.</li>
  <li><code>proposal-to-review</code> / <code>accept-assist-proposal</code> move assist drafts into the human workflow — still not auto-enforce.</li>
</ul>
<p>Details: <code>docs/obligation-review.md</code>. Triage in <a href="#/candidates?band=low">Candidates · low</a>.</p>`,
  },
  {
    id: 'assist',
    title: 'Batch LLM assist & Promptfoo',
    group: 'Governance',
    summary: 'Offline by default; proposals never auto-approve.',
    body: `
<ul>
  <li>Assist runs only on escalated / low-confidence candidates.</li>
  <li>Default CI uses the offline fixture provider ($0). Live compare is credential-gated in <code>llm-assist.yml</code>.</li>
  <li>Proposals stay <code>executable=false</code> until a human accepts them into the review workflow.</li>
  <li>Promptfoo offline suite gates assist quality regressions; scores ≠ assess verdicts.</li>
</ul>
<p>See <a href="#/proposals">Proposals</a>, <a href="#/costs">LLM cost</a>, <a href="#/evals">Evals</a>.</p>`,
  },
  {
    id: 'cli',
    title: 'adrctl command map',
    group: 'Operator reference',
    summary: 'Primary CLI surfaces used by CI and humans.',
    body: `
<table class="docs-table">
  <thead><tr><th>Command</th><th>Role</th></tr></thead>
  <tbody>
    <tr><td><code>validate</code></td><td>Catalog schema / verifier shape.</td></tr>
    <tr><td><code>assess</code> / <code>assess-repo</code></td><td>Issue vs repository assessment (<code>--issue-surface</code>, multi-analyzer evidence).</td></tr>
    <tr><td><code>ingest-adrs</code> / <code>extract-obligations</code></td><td>Markdown → candidates.</td></tr>
    <tr><td><code>assist-obligations</code></td><td>Batch proposals only.</td></tr>
    <tr><td><code>proposal-to-review</code> / <code>accept-assist-proposal</code></td><td>Assist → human review path.</td></tr>
    <tr><td><code>scan-opengrep</code> / <code>scan-checkov</code></td><td>Pinned analyzer runs + evidence.</td></tr>
    <tr><td><code>plan-repo</code> / <code>verify-plan</code> / <code>execute-plan</code></td><td>Deterministic multi-analyzer plans.</td></tr>
    <tr><td><code>review-init</code> / <code>record-obligation-review</code> / <code>assess-obligations</code></td><td>Registry lifecycle.</td></tr>
    <tr><td><code>specification-check</code></td><td>Report-only end-to-end on sample specs.</td></tr>
    <tr><td><code>workshop-emit</code></td><td>Snapshot for Pages.</td></tr>
    <tr><td><code>assert-issue-verdict</code> / <code>low-confidence-checklist</code></td><td>CI / triage helpers.</td></tr>
    <tr><td><code>parse-pr-url</code> / <code>parse-pr-scan-issue</code> / <code>assessment-to-rdjson</code></td><td>PR-scan URL parse + shared rdjson/Security SARIF converter.</td></tr>
  </tbody>
</table>
<p>Usage banner is authoritative in <code>cmd/adrctl/main.go</code>.</p>`,
  },
  {
    id: 'ci-workflows',
    title: 'CI / workflows',
    group: 'Operator reference',
    summary: 'Which Actions compute what, and how Pages refreshes.',
    body: `
<ul>
  <li><strong>CI</strong> — validate, fixtures, analyzer proofs, often <code>workshop-emit</code>.</li>
  <li><strong>ADR Assessment</strong> — issue-triggered sandbox + corpus assess + report comment.</li>
  <li><strong>Pathfinder PR Scan</strong> — PR URL via issue template or <code>workflow_dispatch</code> → reviewdog + optional Security SARIF.</li>
  <li><strong>Pathfinder App PR Scan</strong> — <code>pull_request</code> / dispatch; mints App install token → reusable workflow (<a href="#/docs/github-app">GitHub App</a>).</li>
  <li><strong>Reusable PR Scan</strong> — <code>workflow_call</code> engine for consumers (<code>examples/consumer-pr-scan*.yml</code>).</li>
  <li><strong>Corpus Eval</strong> — labeled harness / extract quality metrics.</li>
  <li><strong>LLM Assist</strong> — batch assist (offline default) + Promptfoo; live gated.</li>
  <li><strong>Workshop Publish</strong> — on <code>workflow_run</code> or <code>workflow_dispatch</code>; merges <code>workshop-data</code> and deploys Pages.</li>
</ul>
<p>Setup map: <a href="#/docs/integrations">Integrations</a>. Bootstrap Pages: Settings → Pages → Source: GitHub Actions. Docs: <code>docs/workshop.md</code> · <code>docs/pr-scan.md</code>.</p>`,
  },
  {
    id: 'workshop-ui',
    title: 'Using this workshop UI',
    group: 'Operator reference',
    summary: 'Hash state, keyboard, exports, drawers, run picker.',
    body: `
<ul>
  <li><strong>Run picker</strong> (top bar) selects which snapshot KPIs/artifacts load (localStorage). Age turns warn when publish data is stale.</li>
  <li><strong>Overview</strong> attention queue deep-links into filtered Candidates / Proposals / Assess; sparklines show recent run trends.</li>
  <li><strong>Hash query state</strong> — shareable filters: <code>q</code>, <code>band</code>, <code>page</code>, <code>rows</code>, <code>sort</code>, <code>dir</code>; Runs compare uses <code>a</code>/<code>b</code>; Proposals parent filter uses <code>parent</code>. Example: <a href="#/candidates?band=low&amp;sort=confidence"><code>#/candidates?band=low</code></a>.</li>
  <li><strong>Keyboard</strong>: <kbd>/</kbd> focuses search, <kbd>j</kbd>/<kbd>k</kbd> or arrows move rows, <kbd>Enter</kbd> activates (opens details on Proposals/Assess), <kbd>o</kbd> or Space expands the detail drawer, or use the visible <strong>Details</strong> control. <kbd>Esc</kbd> closes drawer / blurs search.</li>
  <li><strong>Sort</strong> — click or Enter/Space on column headers.</li>
  <li><strong>Export</strong> — Copy TSV / Markdown / failing checklist for Issues.</li>
  <li><strong>Docs</strong> route as <code>#/docs/:section</code> (this page).</li>
</ul>
<p>Routes: Overview, Metrics, <a href="#/confidence">Confidence</a>, <a href="#/adr-flags">ADR Flagging</a>, Pipeline, Sample, ADRs, Candidates, Proposals, Assess, Costs, Runs, Evals, Corpus, Docs.</p>`,
  },
  {
    id: 'assess-outcomes',
    title: 'Assess outcomes & binding funnel',
    group: 'Operator reference',
    summary: 'N/A rate, PASS/FAIL/IND shares, and inventory→bound→evidence-complete rates.',
    body: `
<p>Dashboard: <a href="#/assess">Assess</a> · <a href="#/metrics?focus=assess">Metrics → assess</a>. Emitted from assessment <code>counts</code> / <code>corpus_coverage</code>.</p>
<h3>Outcome mix</h3>
<ul>
  <li><code>kpi.assess.na_rate</code> — NOT_APPLICABLE / requirement_count (issue-surface batches are often high N/A).</li>
  <li><code>kpi.assess.applicable_count</code> — non-N/A requirements (denominator for shares).</li>
  <li><code>kpi.assess.pass_share</code> / <code>fail_share</code> / <code>indeterminate_share</code> / <code>warn_share</code> — among applicable only.</li>
  <li><code>kpi.assess.submission_kind</code> — <code>tech_spec</code>, <code>code_snippet</code>, or <code>mixed</code> for multi-kind batches.</li>
</ul>
<h3>Corpus coverage funnel</h3>
<p><code>kpi.assess.funnel.*</code> stages: inventory → accepted → interpreted → bound → evidence_complete (counts + rates vs inventory). Binding covered/unbound/stale remain separate obligation-coverage KPIs — not “green estate.”</p>
<p>Related: <a href="#/docs/statuses">Statuses</a> · <a href="#/docs/kpis">KPI glossary</a> · <a href="#/docs/adr-flagging">ADR flagging</a>.</p>`,
  },
  {
    id: 'rule-hits',
    title: 'Rule / analyzer hit rates',
    group: 'Operator reference',
    summary: 'OpenGrep/Checkov rule_id hit and flag counts from assessment findings.',
    body: `
<p>Artifact: <code>rule_hits.json</code> from <code>adrctl workshop-emit</code>. UI table on <a href="#/adr-flags">ADR Flagging</a>; summary KPIs <code>kpi.rule.*</code> on Metrics · assess.</p>
<ul>
  <li><strong>Hit</strong> — a finding with a non-empty <code>rule_id</code>.</li>
  <li><strong>Flag</strong> — a hit on a requirement classified as deviate (FAIL/WARN/INDETERMINATE/…).</li>
  <li><code>kpi.rule.top_rule_id</code> / <code>top_flag_rule_id</code> — hottest rules by hits / deviate-linked hits.</li>
</ul>
<p>Issue-surface assessments often have zero analyzer findings (rules N/A); repository assess runs populate this table.</p>
<p>Related: <a href="#/docs/opengrep">OpenGrep</a> · <a href="#/docs/checkov">Checkov</a> · <a href="#/docs/adr-flagging">ADR flagging</a>.</p>`,
  },
  {
    id: 'adr-flagging',
    title: 'ADR flagging analytics',
    group: 'Operator reference',
    summary: 'Aligned vs deviate definitions, flag rate, and leaderboard KPIs.',
    body: `
<p>Dashboard: <a href="#/adr-flags"><code>#/adr-flags</code></a> (alias <code>#/adrs/insights</code>). Artifact: <code>adr_flagging.json</code> from <code>adrctl workshop-emit</code> (single assessment requirements or <code>--submissions</code> batch index). Summary KPIs land on <code>metrics.json</code> as <code>kpi.adr.*</code>.</p>
<h3>Aligned vs deviate</h3>
<table class="docs-table">
  <thead><tr><th>Class</th><th>Requirement statuses</th><th>In flag rate?</th></tr></thead>
  <tbody>
    <tr><td><strong>Aligned</strong></td><td><code>PASS</code></td><td>Yes (denominator + success)</td></tr>
    <tr><td><strong>Deviated</strong></td><td><code>FAIL</code>, <code>WARN</code>, <code>INDETERMINATE</code>, <code>EVIDENCE_MISSING</code>, <code>ERROR</code>, <code>REVIEW_REQUIRED</code>, <code>UNSUPPORTED</code></td><td>Yes (denominator + flag)</td></tr>
    <tr><td><strong>Excluded</strong></td><td><code>NOT_APPLICABLE</code> (or empty)</td><td>No — counted as touched only</td></tr>
  </tbody>
</table>
<p><strong>Assessed</strong> = aligned + deviate. <strong>Flag rate</strong> = deviate / assessed (Wilson 95% <code>kpi.adr.flag_rate.ci_*</code> when assessed &gt; 0). Issue-surface batches that are all <code>NOT_APPLICABLE</code> show touched ADRs with no flag rate until repository evidence produces PASS/FAIL.</p>
<h3>KPI strip</h3>
<ul>
  <li><code>kpi.adr.touched</code> / <code>kpi.adr.assessed</code> — ADRs with any observation / with ≥1 assessed observation.</li>
  <li><code>kpi.adr.flag_rate</code> (± CI) — overall deviate share among assessed requirement observations.</li>
  <li><code>kpi.adr.top_flag_rate</code> / <code>kpi.adr.top_flag_rate_adr</code> — highest per-ADR flag rate.</li>
  <li><code>kpi.adr.fail_count</code> / <code>pass_count</code> / <code>indeterminate_count</code> — observation totals.</li>
  <li><code>kpi.adr.most_fail</code> / <code>kpi.adr.most_aligned</code> — ADR ids for the strip.</li>
</ul>
<p>Per-ADR dig-in (dashboard expand or <a href="#/adrs">ADRs</a> detail) lists aligned runs, deviate runs, OpenGrep/Checkov rule ids that fired, and deep links into Assess / Submissions / Runs. Split by <code>submission_kind</code> when present.</p>
<p>Related: <a href="#/docs/statuses">Statuses</a> · <a href="#/docs/kpis">KPI glossary</a> · repo <code>docs/kpi-dictionary.md</code>.</p>`,
  },
  {
    id: 'confidence',
    title: 'Confidence scoring (statistical)',
    group: 'Operator reference',
    summary: 'Heuristic triage vs calibrated P(correct) vs conformal bands — honesty limits included.',
    body: `
<p>Pathfinder confidence is produced on the <strong>extract</strong> path (deterministic, zero LLM). Assist gates on the resulting band; assess/workshop metrics carry the same fields forward. Dashboard: <a href="#/confidence"><code>#/confidence</code></a>.</p>
<h3>Three layers (do not conflate)</h3>
<table class="docs-table">
  <thead><tr><th>Field</th><th>Meaning</th><th>Not a claim of…</th></tr></thead>
  <tbody>
    <tr><td><code>confidence_p</code></td><td>Calibrated <em>P(extract_correct)</em> via Platt/logistic fitted on gold. Emit this — not a raw heuristic score.</td><td>Compliance, assess PASS, or Promptfoo quality.</td></tr>
    <tr><td><code>confidence</code> / band</td><td>Inductive conformal prediction set mapped to high / medium / low at target coverage (default 90%). Singleton <code>{high}</code> → high; otherwise widen.</td><td>Guaranteed correctness on shifted domains without refit.</td></tr>
    <tr><td><code>confidence_method</code> / <code>coverage</code></td><td><code>platt_conformal</code> with gold-fitted params (or labeled <code>heuristic</code> only if params missing).</td><td>That fallback heuristic equals a calibrated probability.</td></tr>
  </tbody>
</table>
<p>Heuristic triage from modality / pattern / ambiguity still runs <strong>internally</strong> as one feature for calibration. It is <strong>not</strong> emitted on candidates (the old <code>confidence_score</code> field is removed). Submitters never supply confidence — extract computes it.</p>
<h3>Where scores appear</h3>
<ol class="docs-steps">
  <li><strong>Extract / candidates</strong> — full fields on each candidate + <code>calibration</code> meta on the index.</li>
  <li><strong>Assist</strong> — filters by conformal/heuristic band (<code>max_confidence</code>, default low); proposals copy parent <code>confidence_*</code>.</li>
  <li><strong>Assess</strong> — candidate assessments inherit confidence; optional analyzer <code>evidence_weight</code> from fixture precision-by-rule-id.</li>
  <li><strong>Metrics</strong> — band shares, escalation, mean_p, method counts, Wilson 95% CIs (<code>kpi.*.ci_low/ci_high</code>).</li>
</ol>
<h3>Gold &amp; params</h3>
<ul>
  <li>Gold schema: <code>testdata/confidence/gold-extract-v1.json</code> (<code>extract_correct</code>, <code>expected_band</code>, assess/assist hooks).</li>
  <li>Fitted params (embedded + mirrored): <code>internal/obligations/calibration/params-v1.json</code>.</li>
  <li><strong>Exchangeability:</strong> conformal coverage assumes future candidates are exchangeable with gold. Grow gold (e.g. 50-submissions pack) and refit before claiming the same coverage on a new domain.</li>
</ul>
<h3>How to read bands &amp; CIs</h3>
<ul>
  <li><strong>Band</strong> — triage / assist eligibility, not a compliance verdict.</li>
  <li><strong>Wilson CI</strong> — sampling uncertainty on rates (escalation, shares, harness precision when counts exist). Not calibration error bars on <code>confidence_p</code>.</li>
  <li>Prefer <a href="#/confidence">Confidence</a> for overview + trends; <a href="#/metrics?focus=confidence">Metrics → confidence</a> for raw KPI keys.</li>
</ul>
<p>Related: <a href="#/docs/kpis">KPI glossary</a> · <a href="#/sample/extract">Sample extract step</a> · repo <code>docs/kpi-dictionary.md</code> · <code>docs/architecture.md</code>.</p>`,
  },
  {
    id: 'kpis',
    title: 'KPI dictionary (workshop)',
    group: 'Operator reference',
    summary: 'What the metrics mean — and what they are not.',
    body: `
<ul>
  <li><code>kpi.invariant.llm_calls</code> must be <strong>0</strong> on assess paths.</li>
  <li>Confidence / escalation / harness precision-recall measure extract quality. See <a href="#/docs/confidence">Confidence scoring</a> for calibrated P vs heuristic vs conformal (params version, gold size, empirical coverage).</li>
  <li>Wilson intervals: <code>kpi.*.ci_low</code> / <code>ci_high</code> (95%) on escalation, band shares (incl. medium), harness precision when counts exist. Dashboard: <a href="#/confidence">Confidence</a>.</li>
  <li><code>kpi.assess.submission_kind</code> (<code>tech_spec</code> / <code>code_snippet</code> / <code>mixed</code>) and kind-split mean_p / gap / harness P/R. See <a href="#/docs/assess-outcomes">Assess outcomes</a>.</li>
  <li>Assess outcome mix: <code>kpi.assess.na_rate</code>, applicable count, PASS/FAIL/IND/WARN shares; funnel <code>kpi.assess.funnel.*</code>.</li>
  <li>Assist cost, <code>kpi.cost.per_accepted_proposal</code>, reject rate — batch assist economics, not compliance.</li>
  <li>Promptfoo pass rate is assist regression only.</li>
  <li>Assess covered / unbound / stale track binding completeness, not “green estate.”</li>
  <li><code>kpi.adr.*</code> — ADR flagging. <code>kpi.rule.*</code> — analyzer hit rates (<a href="#/docs/rule-hits">Rule hits</a>). Dashboard: <a href="#/adr-flags">ADR Flagging</a>.</li>
  <li>Publish freshness SLO: Overview tile + <code>latest.slo.publish_freshness_*</code>.</li>
  <li>Every metric tile has an ℹ control with What / Why / How and a Docs link.</li>
</ul>
<p>Authoritative definitions: repository <code>docs/kpi-dictionary.md</code>. Charts: <a href="#/metrics">Metrics</a> · <a href="#/confidence">Confidence</a> · <a href="#/adr-flags">ADR Flagging</a>.</p>`,
  },
  {
    id: 'glossary',
    title: 'Glossary',
    group: 'Operator reference',
    summary: 'Shared vocabulary for Pathfinder surfaces.',
    body: `
<table class="docs-table">
  <thead><tr><th>Term</th><th>Meaning</th></tr></thead>
  <tbody>
    <tr><td>ADR</td><td>Architecture Decision Record (Markdown source of obligations).</td></tr>
    <tr><td>Obligation / candidate</td><td>Cited normative statement extracted from an ADR; review-required until bound.</td></tr>
    <tr><td>Pattern</td><td>Reusable verification family (e.g. <code>security.injection</code>) mapped to verifier types.</td></tr>
    <tr><td>Binding</td><td>Approved link from obligation → pattern/verifier with digests; can go stale.</td></tr>
    <tr><td>Issue surface</td><td><code>assess-repo --issue-surface</code>: sandbox fences; manual-review → N/A.</td></tr>
    <tr><td>Coverage</td><td><code>COMPLETE</code> / <code>PARTIAL</code> / <code>NONE</code> for applicable evidence — not a verdict.</td></tr>
    <tr><td>Escalation</td><td>Low conformal/heuristic band candidates flagged for assist or human review.</td></tr>
    <tr><td>Calibrated P</td><td><code>confidence_p</code> — Platt/logistic P(extract_correct); see <a href="#/docs/confidence">Confidence scoring</a>.</td></tr>
    <tr><td>Conformal band</td><td>Prediction-set triage at target coverage under exchangeability with gold.</td></tr>
    <tr><td>Assist proposal</td><td>Batch LLM draft; <code>executable=false</code> until human accepts into review.</td></tr>
    <tr><td>Plan manifest</td><td>Deterministic analyzer work description; verified before <code>execute-plan</code>.</td></tr>
    <tr><td>workshop-data</td><td>Git branch storing run snapshots and <code>runs.jsonl</code> for Pages.</td></tr>
    <tr><td>GitHub App token</td><td>Short-lived installation access token (≤1h) from <code>APP_ID</code>/<code>APP_CLIENT_ID</code> + <code>APP_PRIVATE_KEY</code>; preferred over PAT for PR scan.</td></tr>
    <tr><td>pathfinder-pr-scan</td><td>Stable reviewdog check name for branch protection.</td></tr>
  </tbody>
</table>`,
  },
  {
    id: 'limitations',
    title: 'Limitations & honesty',
    group: 'Operator reference',
    summary: 'What Pathfinder does not claim.',
    body: `
<ul>
  <li>No execution of issue-submitted code.</li>
  <li>Analyzers prove only their configured rules and scopes.</li>
  <li>Manual / business-semantic obligations remain human-gated in repository mode.</li>
  <li>Joern and deeper graph analysis are not implemented yet (N=10 gate).</li>
  <li>Partial coverage must never be marketed as a full PASS.</li>
  <li>Promptfoo / harness scores are not compliance verdicts.</li>
  <li>Cross-repo PR scan / Security SARIF upload needs a PAT (or App) with write access on the <em>target</em> repo; default <code>GITHUB_TOKEN</code> cannot post elsewhere.</li>
</ul>`,
  },
];

export function docsById(id: string): DocSection | undefined {
  return DOC_SECTIONS.find((s) => s.id === id);
}

export function docsGroups(): Array<{ group: string; items: DocSection[] }> {
  const order: string[] = [];
  const map = new Map<string, DocSection[]>();
  for (const s of DOC_SECTIONS) {
    if (!map.has(s.group)) {
      order.push(s.group);
      map.set(s.group, []);
    }
    map.get(s.group)!.push(s);
  }
  return order.map((group) => ({ group, items: map.get(group)! }));
}
