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
<p>Related: <a href="#/docs/architecture">Architecture</a> · <a href="#/docs/lifecycle">Spec → enforcement</a> · <a href="#/sample">Sample journey</a></p>`,
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
<p>Humans write back via Issues / PR templates / <code>workflow_dispatch</code>. The static site never holds secrets. Local: <code>cd workshop &amp;&amp; npm ci &amp;&amp; npm run dev</code>.</p>`,
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
  <li><strong>Corpus Eval</strong> — labeled harness / extract quality metrics.</li>
  <li><strong>LLM Assist</strong> — batch assist (offline default) + Promptfoo; live gated.</li>
  <li><strong>Workshop Publish</strong> — on <code>workflow_run</code> or <code>workflow_dispatch</code>; merges <code>workshop-data</code> and deploys Pages.</li>
</ul>
<p>Dispatch from chrome <strong>Publish</strong> / <strong>Assist</strong>, or from Overview. Bootstrap: Settings → Pages → Source: GitHub Actions. Doc: <code>docs/workshop.md</code>.</p>`,
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
<p>Routes: Overview, Metrics, <a href="#/confidence">Confidence</a>, Pipeline, Sample, ADRs, Candidates, Proposals, Assess, Costs, Runs, Evals, Corpus, Docs.</p>`,
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
  <li>Confidence / escalation / harness precision-recall measure extract quality. See <a href="#/docs/confidence">Confidence scoring</a> for calibrated P vs heuristic vs conformal.</li>
  <li>Wilson intervals: <code>kpi.*.ci_low</code> / <code>ci_high</code> (95%) on escalation, shares, harness precision when counts exist. Dashboard: <a href="#/confidence">Confidence</a>.</li>
  <li><code>kpi.assess.submission_kind</code> and <code>kpi.submission.*_count</code> / <code>kpi.escalation.rate.{tech_spec,code_snippet}</code> split trends for English tech specs vs code/IaC.</li>
  <li>Assist cost &amp; proposal counts measure batch assist — not compliance.</li>
  <li>Promptfoo pass rate is assist regression only.</li>
  <li>Assess covered / unbound / stale track binding completeness, not “green estate.”</li>
</ul>
<p>Authoritative definitions: repository <code>docs/kpi-dictionary.md</code>. Charts: <a href="#/metrics">Metrics</a> · <a href="#/confidence">Confidence</a>.</p>`,
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
