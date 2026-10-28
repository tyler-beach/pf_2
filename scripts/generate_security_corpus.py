#!/usr/bin/env python3
"""Generate the OWASP + AWS MADR labeled corpus (200 ADRs).

Source of truth is build_taxonomy() below. Running this script writes:
  - testdata/corpus/taxonomy.yml
  - testdata/corpus/adrs/ADR-*.md
  - testdata/corpus/labels.json
  - testdata/corpus/corpus_index.json

  python3 scripts/generate_security_corpus.py
"""

from __future__ import annotations

import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CORPUS = ROOT / "testdata" / "corpus"
ADR_DIR = CORPUS / "adrs"
LABELS_PATH = CORPUS / "labels.json"
TAXONOMY_PATH = CORPUS / "taxonomy.yml"
INDEX_PATH = CORPUS / "corpus_index.json"

# Band / modality / pattern combinations tuned for internal/obligations scoring.
# Decision prose uses RFC 2119 keywords (MUST/SHALL/SHOULD/MAY); modality labels
# stay lowercase for the extract harness contract.
HIGH_CFG = ("high", "must", "configuration.bounds_or_required_values", False)
HIGH_SEC = ("high", "must_not", "security.source_pattern", False)
HIGH_ARCH = ("high", "must_not", "architecture.dependency_or_boundary", False)
MED_SEC = ("medium", "must_not", "security.source_pattern", False)
LOW_REV = ("low", "should", "manual_review", True)
LOW_MAY = ("low", "may", "manual_review", True)

# Keep this free of bare normative keywords so extract ignores Context.
RFC2119_NOTE = (
    "Normative language in the Decision section follows RFC 2119 "
    "(Bradner, S., \"Key words for use in RFCs to Indicate Requirement Levels\", "
    "BCP 14, March 1997)."
)


def _yaml_escape(value: str) -> str:
    if re.search(r'[:#\[\]{},&*?|>!%@`]', value) or value.startswith((" ", "-", '"', "'")):
        return json.dumps(value)
    return value


def dump_taxonomy_yaml(entries: list[dict]) -> str:
    lines = [
        "# OWASP Top 10 + AWS security MADR corpus taxonomy.",
        "# Regenerate ADRs/labels with: python3 scripts/generate_security_corpus.py",
        f"corpus_size: {len(entries)}",
        "entries:",
    ]
    for e in entries:
        lines.append(f"  - id: {e['id']}")
        lines.append(f"    title: {_yaml_escape(e['title'])}")
        lines.append(f"    band: {e['band']}")
        lines.append(f"    modality: {e['modality']}")
        lines.append(f"    pattern_id: {e['pattern_id']}")
        lines.append(f"    escalate: {'true' if e['escalate'] else 'false'}")
        if e.get("owasp"):
            lines.append(f"    owasp: [{', '.join(e['owasp'])}]")
        if e.get("aws"):
            lines.append(f"    aws: [{', '.join(e['aws'])}]")
        lines.append(f"    marker: {_yaml_escape(e['marker'])}")
        lines.append(f"    decision: {_yaml_escape(e['decision'])}")
        lines.append(f"    context: {_yaml_escape(e['context'])}")
        lines.append(f"    consequences: {_yaml_escape(e['consequences'])}")
    return "\n".join(lines) + "\n"


def render_madr(entry: dict) -> str:
    # No YAML frontmatter: adringest requires the first heading to be level-one.
    # Tags live in taxonomy.yml / corpus_index.json / labels.json.
    context = f"{RFC2119_NOTE}\n\n{entry['context']}"
    return (
        f"# {entry['id']}: {entry['title']}\n\n"
        "## Status\n\nAccepted\n\n"
        f"## Context\n\n{context}\n\n"
        f"## Decision\n\n{entry['decision']}\n\n"
        f"## Consequences\n\n{entry['consequences']}\n"
    )


def apply_rfc2119(decision: str, modality: str, adr_id: str) -> str:
    """Rewrite normative cues as uppercase RFC 2119 keywords.

    Hard requirements alternate MUST/SHALL (and MUST NOT/SHALL NOT) by ADR id
    so the corpus exercises both requirement forms.
    """
    use_shall = sum(ord(c) for c in adr_id) % 2 == 0
    if modality == "must":
        pos = "SHALL" if use_shall else "MUST"
        decision = re.sub(r"\bmust\b", pos, decision, count=1, flags=re.I)
    elif modality == "must_not":
        neg = "SHALL NOT" if use_shall else "MUST NOT"
        decision = re.sub(
            r"\bmust not\b",
            neg,
            decision,
            count=1,
            flags=re.I,
        )
    elif modality == "should":
        decision = re.sub(r"\bshould\b", "SHOULD", decision, count=1, flags=re.I)
    elif modality == "may":
        decision = re.sub(r"\bmay\b", "MAY", decision, count=1, flags=re.I)
    return decision


_MODALITY_WORDS = re.compile(
    r"\b(must|shall|should|may|might|required|requires|never|cannot|can't|forbidden|"
    r"prohibited|permitted|permit|do not|does not|don't|need to|has to|have to)\b",
    re.I,
)


def _scrub_non_decision(text: str) -> str:
    """Keep Context/Consequences free of normative cues so extract ignores them."""
    cleaned = _MODALITY_WORDS.sub("is expected to", text)
    cleaned = re.sub(r"\bis expected to not\b", "avoids", cleaned, flags=re.I)
    return cleaned


def _entry(
    adr_id: str,
    title: str,
    band_spec: tuple,
    marker: str,
    decision: str,
    context: str,
    consequences: str,
    *,
    owasp: list[str] | None = None,
    aws: list[str] | None = None,
    pattern_override: str | None = None,
) -> dict:
    band, modality, pattern_id, escalate = band_spec
    if pattern_override:
        pattern_id = pattern_override
    assert marker in decision, (adr_id, marker, decision)
    decision = apply_rfc2119(decision, modality, adr_id)
    assert marker in decision, (adr_id, marker, decision)
    return {
        "id": adr_id,
        "title": title,
        "band": band,
        "modality": modality,
        "pattern_id": pattern_id,
        "escalate": escalate,
        "marker": marker,
        "decision": decision,
        "context": _scrub_non_decision(context),
        "consequences": _scrub_non_decision(consequences),
        "owasp": owasp or [],
        "aws": aws or [],
    }


def build_owasp_entries() -> list[dict]:
    """120 ADRs: 12 per OWASP Top 10 category (A01–A10)."""
    out: list[dict] = []
    start = 3001

    # Each category: 5 high, 3 medium, 4 low ≈ 12 (40%/30%/30% overall with AWS)
    categories: list[tuple[str, str, list[tuple]]] = [
        (
            "A01",
            "Broken Access Control",
            [
                (
                    HIGH_ARCH,
                    "object-level authorization check for resource family {n}",
                    "Services must not return or mutate objects without an object-level authorization check for resource family {n}.",
                    "Missing object-level checks allow horizontal privilege escalation.",
                    "Denied requests remain observable in access logs.",
                    "architecture.access_control",
                ),
                (
                    HIGH_ARCH,
                    "function-level authorization gate for admin action {n}",
                    "Administrative endpoints must not execute without a function-level authorization gate for admin action {n}.",
                    "Function-level gaps expose privileged operations to authenticated users.",
                    "Authorization failures return 403 without leaking existence details.",
                    "architecture.access_control",
                ),
                (
                    HIGH_SEC,
                    "direct object reference token for record slot {n}",
                    "APIs must not accept an unverified direct object reference token for record slot {n}.",
                    "Insecure direct object references bypass ownership checks.",
                    "Identifiers are mapped server-side after authorization.",
                    "security.access_control",
                ),
                (
                    HIGH_ARCH,
                    "cross-tenant data path for tenancy boundary {n}",
                    "Handlers must not cross a cross-tenant data path for tenancy boundary {n}.",
                    "Multi-tenant platforms fail when tenant scope is optional.",
                    "Tenant IDs come from verified auth context only.",
                    "architecture.access_control",
                ),
                (
                    HIGH_SEC,
                    "mass-assignment field pack for form surface {n}",
                    "Services must not bind an unfiltered mass-assignment field pack for form surface {n}.",
                    "Mass assignment lets clients set role or price fields.",
                    "Allow-lists define writable fields per role.",
                    "security.access_control",
                ),
                (
                    MED_SEC,
                    "elevated privilege cookie for session lane {n}",
                    "Clients must not receive an elevated privilege cookie for session lane {n} unless an exception is explicitly documented.",
                    "Privilege cookies expand blast radius when stolen.",
                    "Prefer short-lived server sessions with step-up auth.",
                    "security.access_control",
                ),
                (
                    MED_SEC,
                    "CORS reflection allow-list for origin set {n}",
                    "Gateways must not use a CORS reflection allow-list for origin set {n} unless an exception is explicitly documented.",
                    "Reflecting Origin enables credentialed cross-site calls.",
                    "Explicit origin allow-lists replace reflection.",
                    "security.access_control",
                ),
                (
                    MED_SEC,
                    "directory traversal path join for file lane {n}",
                    "File handlers must not perform a directory traversal path join for file lane {n} unless an exception is explicitly documented.",
                    "Unvalidated paths read outside the intended root.",
                    "Resolve and confine paths before open.",
                    "security.access_control",
                ),
                (
                    LOW_REV,
                    "prefer centralized policy-as-code for access family {n}",
                    "Teams should prefer centralized policy-as-code for access family {n} when authorization rules grow complex.",
                    "Scattered checks drift across services.",
                    "Policy reviews track ownership of each rule pack.",
                    None,
                ),
                (
                    LOW_REV,
                    "prefer deny-by-default catalogs for capability set {n}",
                    "Platforms should prefer deny-by-default catalogs for capability set {n} where capability sprawl is a risk.",
                    "Allow-by-default catalogs accumulate unused rights.",
                    "Periodic access reviews remain required.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented break-glass role for incident lane {n}",
                    "Operators may adopt a documented break-glass role for incident lane {n} when restoring service during incidents.",
                    "Emergency access needs audited escape hatches.",
                    "Break-glass use pages on-call owners.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented partner impersonation flow for support desk {n}",
                    "Support tools may adopt a documented partner impersonation flow for support desk {n} when assisting end users.",
                    "Impersonation is sensitive and often necessary for support.",
                    "Every impersonation session is time-boxed and logged.",
                    None,
                ),
            ],
        ),
        (
            "A02",
            "Cryptographic Failures",
            [
                (
                    HIGH_CFG,
                    "TLS minimum version setting for listener {n}",
                    "Edge configuration must declare a TLS minimum version setting for listener {n}.",
                    "Legacy TLS enables downgrade and weak ciphers.",
                    "Handshake metrics alert on protocol regressions.",
                    "configuration.tls_baseline",
                ),
                (
                    HIGH_CFG,
                    "encryption-at-rest required key for datastore {n}",
                    "Datastore configuration must include an encryption-at-rest required key for datastore {n}.",
                    "Unencrypted volumes expose data on disk theft.",
                    "Key rotation follows platform KMS policy.",
                    "configuration.crypto_baseline",
                ),
                (
                    HIGH_SEC,
                    "MD5 password digest for credential store {n}",
                    "Services must not store an MD5 password digest for credential store {n}.",
                    "Broken hash algorithms are trivial to crack offline.",
                    "Use modern password KDFs with unique salts.",
                    "security.crypto",
                ),
                (
                    HIGH_SEC,
                    "cleartext credential log line for channel {n}",
                    "Services must not emit a cleartext credential log line for channel {n}.",
                    "Secrets in logs propagate through aggregators.",
                    "Redaction filters cover tokens and passwords.",
                    "security.secrets",
                ),
                (
                    HIGH_CFG,
                    "HSTS max-age configuration for site family {n}",
                    "Public sites must set an HSTS max-age configuration for site family {n}.",
                    "Missing HSTS allows SSL stripping on first visit.",
                    "Preload is evaluated per brand domain.",
                    "configuration.tls_baseline",
                ),
                (
                    MED_SEC,
                    "custom crypto protocol for message lane {n}",
                    "Teams must not invent a custom crypto protocol for message lane {n} unless an exception is explicitly documented.",
                    "Homegrown crypto fails subtly under real adversaries.",
                    "Use vetted libraries and protocols only.",
                    "security.crypto",
                ),
                (
                    MED_SEC,
                    "hard-coded encryption secret for vault slot {n}",
                    "Binaries must not embed a hard-coded encryption secret for vault slot {n} unless an exception is explicitly documented.",
                    "Embedded keys are extractable from artifacts.",
                    "Runtime secret managers supply keys.",
                    "security.secrets",
                ),
                (
                    MED_SEC,
                    "SHA1 integrity checksum for artifact lane {n}",
                    "Release pipelines must not rely on a SHA1 integrity checksum for artifact lane {n} unless an exception is explicitly documented.",
                    "SHA-1 collisions undermine integrity guarantees.",
                    "Prefer SHA-256 or stronger digests.",
                    "security.crypto",
                ),
                (
                    LOW_REV,
                    "prefer envelope encryption for backup set {n}",
                    "Data platforms should prefer envelope encryption for backup set {n} when managing large key estates.",
                    "Per-object keys simplify rotation at scale.",
                    "Master keys stay in KMS/HSM boundaries.",
                    None,
                ),
                (
                    LOW_REV,
                    "prefer forward secrecy cipher suites for mesh {n}",
                    "Service meshes should prefer forward secrecy cipher suites for mesh {n} where protocol support allows.",
                    "Static RSA key exchange weakens past-session secrecy.",
                    "Cipher inventory is reviewed quarterly.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented legacy cipher waiver for partner link {n}",
                    "Integrations may adopt a documented legacy cipher waiver for partner link {n} when a partner cannot upgrade.",
                    "Some B2B partners lag on TLS upgrades.",
                    "Waivers expire and are risk-accepted.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented field-level encryption scope for PII class {n}",
                    "Products may adopt a documented field-level encryption scope for PII class {n} when regulation demands it.",
                    "Not all fields need application-level encryption.",
                    "Key access is separated from data access roles.",
                    None,
                ),
            ],
        ),
        (
            "A03",
            "Injection",
            [
                (
                    HIGH_SEC,
                    "dynamic SQL string concat for query family {n}",
                    "Data access code must not build a dynamic SQL string concat for query family {n}.",
                    "String-built SQL enables classic injection.",
                    "Parameterized queries or query builders are mandatory.",
                    "security.injection",
                ),
                (
                    HIGH_SEC,
                    "shell command string for ops helper {n}",
                    "Services must not compose a shell command string for ops helper {n}.",
                    "Shell injection turns input into remote code execution.",
                    "Prefer argv arrays without a shell.",
                    "security.injection",
                ),
                (
                    HIGH_SEC,
                    "eval of untrusted input for script lane {n}",
                    "Runtimes must not perform eval of untrusted input for script lane {n}.",
                    "Dynamic evaluation executes attacker payloads.",
                    "Use parsers and allow-listed operations instead.",
                    "security.injection",
                ),
                (
                    HIGH_SEC,
                    "LDAP filter concat for directory lookup {n}",
                    "Identity code must not build an LDAP filter concat for directory lookup {n}.",
                    "LDAP injection alters filter logic.",
                    "Encode filter values with directory-safe APIs.",
                    "security.injection",
                ),
                (
                    HIGH_SEC,
                    "OS command passthrough for tool wrapper {n}",
                    "Wrappers must not allow an OS command passthrough for tool wrapper {n}.",
                    "Passthrough arguments become shell metacharacters.",
                    "Validate against an allow-list of verbs.",
                    "security.injection",
                ),
                (
                    MED_SEC,
                    "template injection render for page family {n}",
                    "Servers must not enable a template injection render for page family {n} unless an exception is explicitly documented.",
                    "Server-side template injection yields RCE.",
                    "Logic-less templates or sandboxed engines only.",
                    "security.injection",
                ),
                (
                    MED_SEC,
                    "NoSQL operator injection for document query {n}",
                    "APIs must not accept a NoSQL operator injection for document query {n} unless an exception is explicitly documented.",
                    "JSON operators like $gt alter query intent.",
                    "Strip operator keys from user documents.",
                    "security.injection",
                ),
                (
                    MED_SEC,
                    "xpath concat for XML lookup {n}",
                    "XML services must not build an xpath concat for XML lookup {n} unless an exception is explicitly documented.",
                    "XPath injection bypasses access filters.",
                    "Bind variables through the XML API.",
                    "security.injection",
                ),
                (
                    LOW_REV,
                    "prefer prepared statement coverage for datastore {n}",
                    "Teams should prefer prepared statement coverage for datastore {n} when reviewing data-access PRs.",
                    "Mixed ORM and raw SQL increases miss rate.",
                    "Static checks gate new raw SQL.",
                    None,
                ),
                (
                    LOW_REV,
                    "prefer output encoding libraries for view layer {n}",
                    "Frontends should prefer output encoding libraries for view layer {n} when rendering untrusted text.",
                    "Manual escaping is inconsistently applied.",
                    "Framework defaults stay enabled.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented raw SQL escape hatch for analytics job {n}",
                    "Analytics jobs may adopt a documented raw SQL escape hatch for analytics job {n} when the ORM cannot express the query.",
                    "Complex reporting sometimes needs raw SQL.",
                    "Escape hatches require dual review.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented dangerous-HTML allowlist for CMS block {n}",
                    "CMS platforms may adopt a documented dangerous-HTML allowlist for CMS block {n} when marketing requires rich content.",
                    "Rich content conflicts with strict sanitization.",
                    "Allow-lists are versioned and tested.",
                    None,
                ),
            ],
        ),
        (
            "A04",
            "Insecure Design",
            [
                (
                    HIGH_CFG,
                    "rate-limit configuration for public route {n}",
                    "Edge configuration must declare a rate-limit configuration for public route {n}.",
                    "Missing rate limits enable abuse and credential stuffing.",
                    "Limits are tuned per authenticity risk.",
                    "configuration.abuse_controls",
                ),
                (
                    HIGH_CFG,
                    "business-logic retry budget for workflow {n}",
                    "Workflow engines must set a business-logic retry budget for workflow {n}.",
                    "Unbounded retries amplify costly side effects.",
                    "Idempotency keys accompany mutating steps.",
                    "configuration.bounds_or_required_values",
                ),
                (
                    HIGH_ARCH,
                    "trust boundary bypass for payment step {n}",
                    "Checkout flows must not allow a trust boundary bypass for payment step {n}.",
                    "Skipping verification steps enables fraud.",
                    "State machines enforce required transitions.",
                    "architecture.secure_design",
                ),
                (
                    HIGH_SEC,
                    "unauthenticated state-changing RPC for action {n}",
                    "Services must not expose an unauthenticated state-changing RPC for action {n}.",
                    "CSRF-like gaps appear when auth is optional on writes.",
                    "Mutations require authn plus anti-forgery controls.",
                    "security.secure_design",
                ),
                (
                    HIGH_CFG,
                    "account lockout threshold setting for login family {n}",
                    "Identity services must configure an account lockout threshold setting for login family {n}.",
                    "Unlimited guesses defeat password controls.",
                    "Lockouts alert fraud systems.",
                    "configuration.abuse_controls",
                ),
                (
                    MED_SEC,
                    "security control deferral for launch train {n}",
                    "Teams must not accept a security control deferral for launch train {n} unless an exception is explicitly documented.",
                    "Shipping without controls creates permanent debt.",
                    "Deferred controls get dated tickets and owners.",
                    "security.secure_design",
                ),
                (
                    MED_SEC,
                    "client-only authorization decision for feature {n}",
                    "Products must not rely on a client-only authorization decision for feature {n} unless an exception is explicitly documented.",
                    "Client checks are bypassable.",
                    "Server re-validates every sensitive action.",
                    "security.secure_design",
                ),
                (
                    MED_SEC,
                    "shared mutable cache key for tenant {n}",
                    "Caches must not use a shared mutable cache key for tenant {n} unless an exception is explicitly documented.",
                    "Key collisions leak or clobber tenant data.",
                    "Keys include tenant and object identity.",
                    "security.secure_design",
                ),
                (
                    LOW_REV,
                    "prefer threat modeling workshops for initiative {n}",
                    "Delivery teams should prefer threat modeling workshops for initiative {n} before high-risk launches.",
                    "Late design reviews miss structural issues.",
                    "Outputs feed the ADR backlog.",
                    None,
                ),
                (
                    LOW_REV,
                    "prefer abuse-case reviews for marketplace flow {n}",
                    "Product teams should prefer abuse-case reviews for marketplace flow {n} when incentives invite fraud.",
                    "Happy-path QA misses adversarial misuse.",
                    "Findings map to detection stories.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented risk acceptance for beta surface {n}",
                    "Programs may adopt a documented risk acceptance for beta surface {n} when exposing limited beta features.",
                    "Betas sometimes ship with reduced controls.",
                    "Acceptances expire with GA criteria.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented design trade-study for consistency model {n}",
                    "Architects may adopt a documented design trade-study for consistency model {n} when choosing eventual consistency.",
                    "Consistency choices are contextual.",
                    "Trade-studies cite failure modes.",
                    None,
                ),
            ],
        ),
        (
            "A05",
            "Security Misconfiguration",
            [
                (
                    HIGH_CFG,
                    "debug mode configuration flag for environment {n}",
                    "Production configuration must not enable a debug mode configuration flag for environment {n}.",
                    "Debug modes leak stacks and admin tools.",
                    "Deploy gates reject debug=true in prod.",
                    "configuration.hardening",
                ),
                (
                    HIGH_CFG,
                    "default admin password setting for appliance {n}",
                    "Platform images must rotate any default admin password setting for appliance {n}.",
                    "Default credentials are publicly known.",
                    "First-boot forces unique secrets.",
                    "configuration.hardening",
                ),
                (
                    HIGH_CFG,
                    "unnecessary feature toggle for service profile {n}",
                    "Hardened profiles must disable each unnecessary feature toggle for service profile {n}.",
                    "Extra features expand attack surface.",
                    "Baselines are versioned per runtime.",
                    "configuration.hardening",
                ),
                (
                    HIGH_CFG,
                    "security header required key for HTTP profile {n}",
                    "Edge configuration must include a security header required key for HTTP profile {n}.",
                    "Missing headers weaken browser isolation.",
                    "Header baselines cover CSP frame options and referrer.",
                    "configuration.hardening",
                ),
                (
                    HIGH_CFG,
                    "error verbosity configuration for API family {n}",
                    "APIs must set a bounded error verbosity configuration for API family {n}.",
                    "Verbose errors expose internals to attackers.",
                    "Clients receive opaque correlation IDs.",
                    "configuration.hardening",
                ),
                (
                    MED_SEC,
                    "directory listing enablement for static host {n}",
                    "Static hosts must not allow a directory listing enablement for static host {n} unless an exception is explicitly documented.",
                    "Listings reveal sensitive paths and backups.",
                    "Indexes are disabled by default.",
                    "security.misconfiguration",
                ),
                (
                    MED_SEC,
                    "wildcard CORS credential mode for API {n}",
                    "APIs must not enable a wildcard CORS credential mode for API {n} unless an exception is explicitly documented.",
                    "Wildcard plus credentials is an unsafe browser combo.",
                    "Origins are enumerated explicitly.",
                    "security.misconfiguration",
                ),
                (
                    MED_SEC,
                    "cloud metadata exposure path for workload {n}",
                    "Workloads must not expose a cloud metadata exposure path for workload {n} unless an exception is explicitly documented.",
                    "SSRF to metadata yields cloud credentials.",
                    "IMDSv2 and egress controls are required.",
                    "security.misconfiguration",
                ),
                (
                    LOW_REV,
                    "prefer hardened OS baselines for compute pool {n}",
                    "Platform teams should prefer hardened OS baselines for compute pool {n} when provisioning fleets.",
                    "Ad-hoc images drift quickly.",
                    "Baselines rebuild on a cadence.",
                    None,
                ),
                (
                    LOW_REV,
                    "prefer automated config drift detection for stack {n}",
                    "Ops should prefer automated config drift detection for stack {n} where manual reviews miss changes.",
                    "Silent drift reopens closed findings.",
                    "Drift alerts page owning teams.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented temporary debug session for incident {n}",
                    "Responders may adopt a documented temporary debug session for incident {n} when diagnosing production faults.",
                    "Live debug can be necessary during SEVs.",
                    "Sessions auto-expire and are audited.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented non-prod parity exception for lab {n}",
                    "Labs may adopt a documented non-prod parity exception for lab {n} when full prod hardening blocks testing.",
                    "Some labs need looser settings.",
                    "Exceptions never apply to production accounts.",
                    None,
                ),
            ],
        ),
        (
            "A06",
            "Vulnerable and Outdated Components",
            [
                (
                    HIGH_CFG,
                    "dependency allow-list configuration for runtime {n}",
                    "Build systems must maintain a dependency allow-list configuration for runtime {n}.",
                    "Unvetted packages introduce known CVEs.",
                    "Allow-list updates require security review.",
                    "configuration.supply_chain",
                ),
                (
                    HIGH_ARCH,
                    "unpinned transitive import for module set {n}",
                    "Services must not rely on an unpinned transitive import for module set {n}.",
                    "Floating versions pull breaking or vulnerable releases.",
                    "Lockfiles are committed and verified in CI.",
                    "architecture.supply_chain",
                ),
                (
                    HIGH_CFG,
                    "CVE remediation SLA setting for severity {n}",
                    "Security configuration must declare a CVE remediation SLA setting for severity {n}.",
                    "Without SLAs, critical CVEs linger.",
                    "SLA breaches open executive risk tickets.",
                    "configuration.supply_chain",
                ),
                (
                    HIGH_SEC,
                    "end-of-life runtime package for service {n}",
                    "Deployments must not ship an end-of-life runtime package for service {n}.",
                    "EOL runtimes stop receiving security patches.",
                    "Upgrade trains are scheduled before EOL dates.",
                    "security.supply_chain",
                ),
                (
                    HIGH_CFG,
                    "SBOM required key for release artifact {n}",
                    "Release pipelines must emit an SBOM required key for release artifact {n}.",
                    "Without SBOMs, blast radius of CVEs is unknown.",
                    "SBOMs attach to provenance attestations.",
                    "configuration.supply_chain",
                ),
                (
                    MED_SEC,
                    "unmaintained library retention for legacy app {n}",
                    "Teams must not keep an unmaintained library retention for legacy app {n} unless an exception is explicitly documented.",
                    "Abandoned libraries accumulate exploit PoCs.",
                    "Exceptions include compensating controls.",
                    "security.supply_chain",
                ),
                (
                    MED_SEC,
                    "direct Git dependency URL for build {n}",
                    "Builds must not fetch a direct Git dependency URL for build {n} unless an exception is explicitly documented.",
                    "Mutable Git refs bypass review and hashing.",
                    "Prefer versioned registries with digests.",
                    "security.supply_chain",
                ),
                (
                    MED_SEC,
                    "unsigned package install for agent {n}",
                    "Agents must not perform an unsigned package install for agent {n} unless an exception is explicitly documented.",
                    "Unsigned packages enable supply-chain swaps.",
                    "Signature verification is enforced.",
                    "security.supply_chain",
                ),
                (
                    LOW_REV,
                    "prefer automated dependency update bots for repo {n}",
                    "Teams should prefer automated dependency update bots for repo {n} when patch volume is high.",
                    "Manual bumps lag behind disclosures.",
                    "Bots still require human review.",
                    None,
                ),
                (
                    LOW_REV,
                    "prefer private package mirrors for build farm {n}",
                    "Platforms should prefer private package mirrors for build farm {n} where upstream availability is a risk.",
                    "Public registries can be unavailable or tampered.",
                    "Mirrors verify upstream checksums.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented freeze window for release train {n}",
                    "Programs may adopt a documented freeze window for release train {n} when stabilizing a major launch.",
                    "Freezes delay patches intentionally.",
                    "Critical CVE exceptions pierce freezes.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented vendor risk acceptance for OEM component {n}",
                    "Hardware programs may adopt a documented vendor risk acceptance for OEM component {n} when firmware cannot be patched in-house.",
                    "OEM components constrain remediation.",
                    "Acceptances include vendor SLAs.",
                    None,
                ),
            ],
        ),
        (
            "A07",
            "Identification and Authentication Failures",
            [
                (
                    HIGH_CFG,
                    "password minimum length setting for realm {n}",
                    "Identity configuration must declare a password minimum length setting for realm {n}.",
                    "Short passwords fall to online guessing.",
                    "Compromised-password checks complement length.",
                    "configuration.authentication",
                ),
                (
                    HIGH_SEC,
                    "credential stuffing without bot controls for login {n}",
                    "Login services must not allow credential stuffing without bot controls for login {n}.",
                    "Unthrottled logins enable stuffing at scale.",
                    "Bot and velocity controls are mandatory.",
                    "security.authentication",
                ),
                (
                    HIGH_SEC,
                    "session fixation accept for auth flow {n}",
                    "Auth flows must not perform a session fixation accept for auth flow {n}.",
                    "Reusing pre-login session IDs enables fixation.",
                    "Rotate session identifiers on authentication.",
                    "security.authentication",
                ),
                (
                    HIGH_CFG,
                    "MFA required key for privileged role {n}",
                    "Privileged access configuration must include an MFA required key for privileged role {n}.",
                    "Password-only admin access is insufficient.",
                    "Phishing-resistant factors are preferred.",
                    "configuration.authentication",
                ),
                (
                    HIGH_SEC,
                    "plaintext password recovery channel for user {n}",
                    "Recovery flows must not send a plaintext password recovery channel for user {n}.",
                    "Emailing passwords defeats hashing controls.",
                    "Use single-use reset tokens only.",
                    "security.authentication",
                ),
                (
                    MED_SEC,
                    "long-lived bearer token for SPA session {n}",
                    "SPAs must not store a long-lived bearer token for SPA session {n} unless an exception is explicitly documented.",
                    "Long-lived tokens magnify XSS impact.",
                    "Prefer short-lived access tokens plus refresh rotation.",
                    "security.authentication",
                ),
                (
                    MED_SEC,
                    "knowledge-based auth questions for recovery {n}",
                    "Recovery must not rely on knowledge-based auth questions for recovery {n} unless an exception is explicitly documented.",
                    "KBA answers are often public or reused.",
                    "Out-of-band verified channels replace KBA.",
                    "security.authentication",
                ),
                (
                    MED_SEC,
                    "concurrent session unlimited policy for account {n}",
                    "Accounts must not use a concurrent session unlimited policy for account {n} unless an exception is explicitly documented.",
                    "Unlimited sessions hide account takeover.",
                    "Session inventories support remote revoke.",
                    "security.authentication",
                ),
                (
                    LOW_REV,
                    "prefer phishing-resistant MFA for workforce {n}",
                    "Workforce IAM should prefer phishing-resistant MFA for workforce {n} when rolling out stronger factors.",
                    "OTP MFA is phishable at scale.",
                    "WebAuthn/passkeys are the target state.",
                    None,
                ),
                (
                    LOW_REV,
                    "prefer centralized identity providers for app portfolio {n}",
                    "Product orgs should prefer centralized identity providers for app portfolio {n} when reducing password sprawl.",
                    "Per-app identity multiplies weak configs.",
                    "Federation standards are documented.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented service-account password for legacy batch {n}",
                    "Legacy batches may adopt a documented service-account password for legacy batch {n} when modern federation is unavailable.",
                    "Some mainframe jobs still need passwords.",
                    "Passwords rotate and are vaulted.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented step-up auth exemption for read API {n}",
                    "APIs may adopt a documented step-up auth exemption for read API {n} when low-risk reads would over-prompt users.",
                    "Step-up friction can harm UX on reads.",
                    "Exemptions exclude sensitive fields.",
                    None,
                ),
            ],
        ),
        (
            "A08",
            "Software and Data Integrity Failures",
            [
                (
                    HIGH_SEC,
                    "unsigned update payload for client {n}",
                    "Update agents must not apply an unsigned update payload for client {n}.",
                    "Unsigned updates enable supply-chain takeover.",
                    "Signatures verify against pinned public keys.",
                    "security.integrity",
                ),
                (
                    HIGH_SEC,
                    "insecure deserialization gadget for payload {n}",
                    "Services must not perform an insecure deserialization gadget for payload {n}.",
                    "Unsafe deserializers execute attacker gadgets.",
                    "Prefer safe formats and allow-listed types.",
                    "security.integrity",
                ),
                (
                    HIGH_CFG,
                    "CI provenance required key for deploy artifact {n}",
                    "Deploy pipelines must attach a CI provenance required key for deploy artifact {n}.",
                    "Without provenance, builds can be swapped.",
                    "Attestations are verified before promote.",
                    "configuration.integrity",
                ),
                (
                    HIGH_ARCH,
                    "untrusted plugin import for extension host {n}",
                    "Hosts must not load an untrusted plugin import for extension host {n}.",
                    "Plugins run with host privileges.",
                    "Plugins are signed and permission-scoped.",
                    "architecture.integrity",
                ),
                (
                    HIGH_SEC,
                    "auto-execute macro content for document pipe {n}",
                    "Pipelines must not auto-execute macro content for document pipe {n}.",
                    "Macros become code execution vectors.",
                    "Documents are sandboxed or converted.",
                    "security.integrity",
                ),
                (
                    MED_SEC,
                    "CDN integrity skip for script include {n}",
                    "Pages must not use a CDN integrity skip for script include {n} unless an exception is explicitly documented.",
                    "Missing SRI allows CDN compromise impact.",
                    "Subresource Integrity hashes are mandatory.",
                    "security.integrity",
                ),
                (
                    MED_SEC,
                    "mutable tag deploy reference for release {n}",
                    "Deployments must not use a mutable tag deploy reference for release {n} unless an exception is explicitly documented.",
                    "Floating tags can be retargeted silently.",
                    "Immutable digests pin what runs.",
                    "security.integrity",
                ),
                (
                    MED_SEC,
                    "webhook without signature check for integration {n}",
                    "Integrations must not accept a webhook without signature check for integration {n} unless an exception is explicitly documented.",
                    "Unsigned webhooks are forgeable.",
                    "HMAC or mTLS authenticates publishers.",
                    "security.integrity",
                ),
                (
                    LOW_REV,
                    "prefer reproducible builds for release train {n}",
                    "Release engineering should prefer reproducible builds for release train {n} when verifying supply-chain claims.",
                    "Non-reproducible builds obscure tampering.",
                    "Differences are investigated before ship.",
                    None,
                ),
                (
                    LOW_REV,
                    "prefer signed commits for protected branch {n}",
                    "Repositories should prefer signed commits for protected branch {n} when defending against author spoofing.",
                    "Unsigned history weakens audit trails.",
                    "Signing keys are centrally managed.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented unsigned internal package for lab {n}",
                    "Labs may adopt a documented unsigned internal package for lab {n} when prototyping throwaway tooling.",
                    "Labs move faster with fewer gates.",
                    "Unsigned packages cannot promote to prod.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented integrity downgrade for partner feed {n}",
                    "Integrations may adopt a documented integrity downgrade for partner feed {n} when a partner cannot sign payloads.",
                    "Some partners lack signing capability.",
                    "Compensating network controls are required.",
                    None,
                ),
            ],
        ),
        (
            "A09",
            "Security Logging and Monitoring Failures",
            [
                (
                    HIGH_CFG,
                    "security event logging required key for service {n}",
                    "Service configuration must include a security event logging required key for service {n}.",
                    "Without security events, detections are blind.",
                    "Events cover authn failures and policy denies.",
                    "configuration.logging",
                ),
                (
                    HIGH_CFG,
                    "log retention days setting for audit lane {n}",
                    "Audit configuration must declare a log retention days setting for audit lane {n}.",
                    "Short retention destroys forensics value.",
                    "Retention meets legal and detection needs.",
                    "configuration.logging",
                ),
                (
                    HIGH_SEC,
                    "silent auth failure path for login family {n}",
                    "Identity services must not use a silent auth failure path for login family {n}.",
                    "Unaudited failures hide credential attacks.",
                    "Failures emit structured security events.",
                    "security.logging",
                ),
                (
                    HIGH_CFG,
                    "alerting webhook configuration for detection {n}",
                    "Detection stacks must set an alerting webhook configuration for detection {n}.",
                    "Logs without alerts delay response.",
                    "Pages include runbook links.",
                    "configuration.logging",
                ),
                (
                    HIGH_CFG,
                    "immutable log sink required key for archive {n}",
                    "Security logging must declare an immutable log sink required key for archive {n}.",
                    "Mutable sinks allow attackers to erase trails.",
                    "Sinks enforce append-only controls.",
                    "configuration.logging",
                ),
                (
                    MED_SEC,
                    "PII-rich debug logging for request {n}",
                    "Services must not enable PII-rich debug logging for request {n} unless an exception is explicitly documented.",
                    "Over-logging creates privacy incidents.",
                    "Field allow-lists govern log payloads.",
                    "security.logging",
                ),
                (
                    MED_SEC,
                    "log shipping disablement for node pool {n}",
                    "Fleets must not allow a log shipping disablement for node pool {n} unless an exception is explicitly documented.",
                    "Local-only logs vanish with the node.",
                    "Agents are monitored for heartbeat loss.",
                    "security.logging",
                ),
                (
                    MED_SEC,
                    "clock skew ignore for audit stamp {n}",
                    "Auditors must not accept a clock skew ignore for audit stamp {n} unless an exception is explicitly documented.",
                    "Skew breaks correlation across systems.",
                    "NTP hardening is mandatory.",
                    "security.logging",
                ),
                (
                    LOW_REV,
                    "prefer centralized SIEM correlation for detection pack {n}",
                    "Security teams should prefer centralized SIEM correlation for detection pack {n} when stitching multi-system attacks.",
                    "Siloed logs miss lateral movement.",
                    "Use cases are versioned as code.",
                    None,
                ),
                (
                    LOW_REV,
                    "prefer canary detections for control family {n}",
                    "Detection owners should prefer canary detections for control family {n} when validating pipeline health.",
                    "Silent detection failures go unnoticed.",
                    "Canary misses page pipeline owners.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented reduced logging mode for privacy region {n}",
                    "Products may adopt a documented reduced logging mode for privacy region {n} when local law restricts telemetry.",
                    "Privacy regimes constrain security logs.",
                    "Compensating monitoring is described.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented sampling strategy for high-volume trace {n}",
                    "Observability teams may adopt a documented sampling strategy for high-volume trace {n} when full capture is cost-prohibitive.",
                    "Full fidelity can be unaffordable.",
                    "Security-critical events remain unsampled.",
                    None,
                ),
            ],
        ),
        (
            "A10",
            "Server-Side Request Forgery",
            [
                (
                    HIGH_SEC,
                    "user-controlled URL fetch for connector {n}",
                    "Services must not perform a user-controlled URL fetch for connector {n}.",
                    "SSRF reaches internal and cloud metadata targets.",
                    "Allow-listed hosts and schemes are enforced.",
                    "security.ssrf",
                ),
                (
                    HIGH_SEC,
                    "raw HTTP redirect follow for webhook {n}",
                    "HTTP clients must not enable a raw HTTP redirect follow for webhook {n}.",
                    "Redirects pivot SSRF into unexpected networks.",
                    "Redirects are disabled or re-validated.",
                    "security.ssrf",
                ),
                (
                    HIGH_SEC,
                    "link-local address request for probe {n}",
                    "Egress must not allow a link-local address request for probe {n}.",
                    "Link-local ranges host cloud metadata services.",
                    "Block 169.254.0.0/16 and equivalent IPv6.",
                    "security.ssrf",
                ),
                (
                    HIGH_CFG,
                    "egress allow-list configuration for fetcher {n}",
                    "Fetcher configuration must declare an egress allow-list configuration for fetcher {n}.",
                    "Open egress turns SSRF into intranet scanning.",
                    "Deny-by-default egress is preferred.",
                    "configuration.ssrf",
                ),
                (
                    HIGH_SEC,
                    "file URL scheme open for import job {n}",
                    "Importers must not allow a file URL scheme open for import job {n}.",
                    "file:// URLs read local secrets.",
                    "Only https schemes are permitted.",
                    "security.ssrf",
                ),
                (
                    MED_SEC,
                    "DNS rebinding tolerance for preview fetch {n}",
                    "Previewers must not allow a DNS rebinding tolerance for preview fetch {n} unless an exception is explicitly documented.",
                    "Rebinding races bypass host allow-lists.",
                    "Resolve once and pin the approved IP.",
                    "security.ssrf",
                ),
                (
                    MED_SEC,
                    "internal hostname allow for media proxy {n}",
                    "Media proxies must not grant an internal hostname allow for media proxy {n} unless an exception is explicitly documented.",
                    "Internal hostnames expand SSRF reach.",
                    "Public suffixes only, with IP validation.",
                    "security.ssrf",
                ),
                (
                    MED_SEC,
                    "webhook callback to RFC1918 for integration {n}",
                    "Integrations must not permit a webhook callback to RFC1918 for integration {n} unless an exception is explicitly documented.",
                    "Private callbacks expose internal services.",
                    "Callbacks require public verified endpoints.",
                    "security.ssrf",
                ),
                (
                    LOW_REV,
                    "prefer network egress proxies for URL preview {n}",
                    "Platforms should prefer network egress proxies for URL preview {n} when centralizing SSRF controls.",
                    "Per-service filters drift.",
                    "Proxies emit deny metrics.",
                    None,
                ),
                (
                    LOW_REV,
                    "prefer metadata hop enforcement for cloud workload {n}",
                    "Cloud platforms should prefer metadata hop enforcement for cloud workload {n} when mitigating SSRF credential theft.",
                    "IMDSv1 is trivial via SSRF.",
                    "IMDSv2 hop limits are mandatory.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented internal fetch exception for admin tool {n}",
                    "Admin tools may adopt a documented internal fetch exception for admin tool {n} when operators need to reach internal health endpoints.",
                    "Some admin workflows need internal URLs.",
                    "Exceptions are identity-bound and audited.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented partner callback to private link {n}",
                    "Integrations may adopt a documented partner callback to private link {n} when a partner requires PrivateLink delivery.",
                    "PrivateLink is intentional private reachability.",
                    "Allow-lists are account-scoped.",
                    None,
                ),
            ],
        ),
    ]

    idx = 0
    for owasp_id, _name, specs in categories:
        for slot, spec in enumerate(specs, start=1):
            band_spec, marker_t, decision_t, context, consequences, pattern_over = spec
            n = slot  # unique per category; global uniqueness via adr id + marker text
            # Ensure global unique markers across categories by including owasp id in marker.
            marker = marker_t.format(n=n) + f" ({owasp_id}-{n})"
            # Inject unique suffix into decision while keeping modality keywords.
            decision = decision_t.format(n=n)
            # Append unique tag inside decision without breaking modality parse — include marker phrase.
            # Reconstruct: replace the formatted marker core with the unique marker.
            core = marker_t.format(n=n)
            decision = decision.replace(core, marker, 1)
            adr_id = f"ADR-{start + idx}"
            out.append(
                _entry(
                    adr_id,
                    f"{owasp_id}: {_title_from_marker(marker, owasp_id)}",
                    band_spec,
                    marker,
                    decision,
                    context,
                    consequences,
                    owasp=[owasp_id],
                    pattern_override=pattern_over,
                )
            )
            idx += 1
    assert len(out) == 120, len(out)
    return out


def _title_from_marker(marker: str, prefix: str) -> str:
    base = marker.split(" (")[0]
    return base[:1].upper() + base[1:]


def build_aws_entries() -> list[dict]:
    """80 ADRs across AWS security practice themes."""
    out: list[dict] = []
    start = 3201
    themes: list[tuple[str, list[tuple]]] = [
        (
            "iam",
            [
                (
                    HIGH_ARCH,
                    "wildcard IAM action star for role {n}",
                    "IAM policies must not grant a wildcard IAM action star for role {n}.",
                    "Action wildcards violate least privilege.",
                    "Policies list explicit actions only.",
                    "architecture.aws_iam",
                ),
                (
                    HIGH_ARCH,
                    "Resource star on sensitive IAM policy for role {n}",
                    "IAM policies must not use a Resource star on sensitive IAM policy for role {n}.",
                    "Resource=* widens blast radius.",
                    "Resources are ARNs scoped to the workload.",
                    "architecture.aws_iam",
                ),
                (
                    HIGH_CFG,
                    "IAM Access Analyzer required key for account {n}",
                    "Account baselines must include an IAM Access Analyzer required key for account {n}.",
                    "External access findings need continuous detection.",
                    "Analyzer findings page security owners.",
                    "configuration.aws_iam",
                ),
                (
                    MED_SEC,
                    "inline IAM policy sprawl for role {n}",
                    "Teams must not create an inline IAM policy sprawl for role {n} unless an exception is explicitly documented.",
                    "Inline policies evade reusable review.",
                    "Managed policies are preferred.",
                    "security.aws_iam",
                ),
                (
                    LOW_REV,
                    "prefer permission boundaries for developer role {n}",
                    "Platforms should prefer permission boundaries for developer role {n} when delegating IAM creation.",
                    "Unbounded CreateRole enables privilege escalation.",
                    "Boundaries cap maximum permissions.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented break-glass IAM role for account {n}",
                    "Security teams may adopt a documented break-glass IAM role for account {n} when recovering from lockouts.",
                    "Break-glass access is sometimes required.",
                    "Assumptions are dual-controlled and logged.",
                    None,
                ),
                (
                    HIGH_SEC,
                    "long-term IAM user access key for workload {n}",
                    "Workloads must not use a long-term IAM user access key for workload {n}.",
                    "Static keys leak and rarely rotate.",
                    "Prefer roles and temporary credentials.",
                    "security.aws_iam",
                ),
                (
                    MED_SEC,
                    "sts assume-role without external ID for partner {n}",
                    "Cross-account trusts must not allow sts assume-role without external ID for partner {n} unless an exception is explicitly documented.",
                    "Missing external IDs enable confused deputy.",
                    "ExternalId conditions are mandatory for partners.",
                    "security.aws_iam",
                ),
            ],
        ),
        (
            "encryption",
            [
                (
                    HIGH_CFG,
                    "KMS CMK required key for data class {n}",
                    "Sensitive data stores must declare a KMS CMK required key for data class {n}.",
                    "AWS-managed defaults may not meet separation needs.",
                    "CMK policies separate admins from users.",
                    "configuration.aws_encryption",
                ),
                (
                    HIGH_CFG,
                    "TLS in transit required key for service link {n}",
                    "Service links must set a TLS in transit required key for service link {n}.",
                    "Cleartext AWS service calls expose data.",
                    "SDK minimum TLS is enforced.",
                    "configuration.aws_encryption",
                ),
                (
                    HIGH_SEC,
                    "SSE-S3 demotion for regulated bucket {n}",
                    "Regulated buckets must not allow an SSE-S3 demotion for regulated bucket {n}.",
                    "Customer-controlled keys may be required.",
                    "SSE-KMS with CMKs is the baseline.",
                    "security.aws_encryption",
                ),
                (
                    MED_SEC,
                    "plaintext Secrets Manager put for secret {n}",
                    "Automation must not perform a plaintext Secrets Manager put for secret {n} unless an exception is explicitly documented.",
                    "CLI mistakes leave secrets in shell history.",
                    "Prefer API calls from secured runners.",
                    "security.aws_encryption",
                ),
                (
                    LOW_REV,
                    "prefer automatic KMS key rotation for CMK {n}",
                    "Security teams should prefer automatic KMS key rotation for CMK {n} when keys guard long-lived data.",
                    "Manual rotation is often skipped.",
                    "Apps must tolerate key version changes.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented client-side encryption for extranet file {n}",
                    "Partners may adopt a documented client-side encryption for extranet file {n} when sharing regulated files.",
                    "Some transfers need encryption before upload.",
                    "Key exchange procedures are documented.",
                    None,
                ),
                (
                    HIGH_CFG,
                    "EBS encryption by default setting for account {n}",
                    "Accounts must enable an EBS encryption by default setting for account {n}.",
                    "Unencrypted volumes appear via misconfiguration.",
                    "SCPs reinforce the account setting.",
                    "configuration.aws_encryption",
                ),
                (
                    MED_SEC,
                    "disabled RDS encryption flag for database {n}",
                    "Databases must not ship a disabled RDS encryption flag for database {n} unless an exception is explicitly documented.",
                    "Unencrypted RDS snapshots leak easily.",
                    "Encryption is enabled at create time.",
                    "security.aws_encryption",
                ),
            ],
        ),
        (
            "network",
            [
                (
                    HIGH_ARCH,
                    "public subnet placement for data store {n}",
                    "Data stores must not use a public subnet placement for data store {n}.",
                    "Public subnets expose databases to the internet path.",
                    "Private subnets plus controlled ingress only.",
                    "architecture.aws_network",
                ),
                (
                    HIGH_CFG,
                    "VPC flow logs required key for VPC {n}",
                    "Network baselines must include a VPC flow logs required key for VPC {n}.",
                    "Without flow logs, lateral movement is hard to see.",
                    "Logs land in immutable accounts.",
                    "configuration.aws_network",
                ),
                (
                    HIGH_SEC,
                    "0.0.0.0/0 SSH ingress for security group {n}",
                    "Security groups must not allow a 0.0.0.0/0 SSH ingress for security group {n}.",
                    "Open SSH is routinely scanned and exploited.",
                    "Admin access uses SSM or tightly scoped CIDRs.",
                    "security.aws_network",
                ),
                (
                    MED_SEC,
                    "NACL allow-all ephemeral rule for VPC {n}",
                    "Teams must not rely on a NACL allow-all ephemeral rule for VPC {n} unless an exception is explicitly documented.",
                    "Over-broad NACLs weaken defense in depth.",
                    "NACLs complement security groups deliberately.",
                    "security.aws_network",
                ),
                (
                    LOW_REV,
                    "prefer centralized egress inspection for VPC {n}",
                    "Network teams should prefer centralized egress inspection for VPC {n} when controlling data exfiltration.",
                    "Per-VPC ad-hoc NAT hides destinations.",
                    "Egress gateways log and filter destinations.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented public ALB for marketing site {n}",
                    "Web teams may adopt a documented public ALB for marketing site {n} when content is intentionally public.",
                    "Public ingress is valid for public sites.",
                    "WAF and TLS baselines still apply.",
                    None,
                ),
                (
                    HIGH_ARCH,
                    "peering without least-privilege routes for VPC {n}",
                    "VPC peerings must not use a peering without least-privilege routes for VPC {n}.",
                    "Full-net routes overexpose peered CIDRs.",
                    "Routes advertise only required prefixes.",
                    "architecture.aws_network",
                ),
                (
                    MED_SEC,
                    "security group reference cycles for tier {n}",
                    "Architectures must not create security group reference cycles for tier {n} unless an exception is explicitly documented.",
                    "Cycles obscure effective permissions.",
                    "Tiered SG designs are documented.",
                    "security.aws_network",
                ),
            ],
        ),
        (
            "logging",
            [
                (
                    HIGH_CFG,
                    "CloudTrail multi-region required key for org {n}",
                    "Organizations must enable a CloudTrail multi-region required key for org {n}.",
                    "Partial trails miss regional attacker activity.",
                    "Org trails bind to a security Log Archive account.",
                    "configuration.aws_logging",
                ),
                (
                    HIGH_CFG,
                    "CloudTrail log file validation setting for trail {n}",
                    "Trails must enable a CloudTrail log file validation setting for trail {n}.",
                    "Without validation, log tampering is harder to detect.",
                    "Validation digests are monitored.",
                    "configuration.aws_logging",
                ),
                (
                    HIGH_SEC,
                    "CloudTrail stop-logging permission for role {n}",
                    "Operational roles must not hold a CloudTrail stop-logging permission for role {n}.",
                    "Attackers stop trails to hide activity.",
                    "Only break-glass roles may alter trails.",
                    "security.aws_logging",
                ),
                (
                    MED_SEC,
                    "S3 access logging disablement for trail bucket {n}",
                    "Trail buckets must not allow an S3 access logging disablement for trail bucket {n} unless an exception is explicitly documented.",
                    "Access logs detect unauthorized reads of trails.",
                    "Logging buckets are separately locked down.",
                    "security.aws_logging",
                ),
                (
                    LOW_REV,
                    "prefer GuardDuty across all regions for account {n}",
                    "Security teams should prefer GuardDuty across all regions for account {n} when enabling threat detection.",
                    "Single-region detectors miss elsewhere.",
                    "Findings aggregate to a delegated admin.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented CloudTrail exclusion for high-volume data event {n}",
                    "Platforms may adopt a documented CloudTrail exclusion for high-volume data event {n} when costs would dominate.",
                    "Data events can be extremely high volume.",
                    "Exclusions are risk-reviewed annually.",
                    None,
                ),
                (
                    HIGH_CFG,
                    "Config recorder required key for region {n}",
                    "Regions must enable a Config recorder required key for region {n}.",
                    "Without Config, drift and noncompliance hide.",
                    "Rules evaluate continuous compliance.",
                    "configuration.aws_logging",
                ),
                (
                    MED_SEC,
                    "CloudWatch log group without retention for app {n}",
                    "Apps must not create a CloudWatch log group without retention for app {n} unless an exception is explicitly documented.",
                    "Infinite retention explodes cost; zero loses forensics.",
                    "Retention matches the data classification.",
                    "security.aws_logging",
                ),
            ],
        ),
        (
            "secrets",
            [
                (
                    HIGH_SEC,
                    "hardcoded AWS access key in source for app {n}",
                    "Repositories must not contain a hardcoded AWS access key in source for app {n}.",
                    "Committed keys are harvested by scanners within minutes.",
                    "Secrets scanners gate merges.",
                    "security.aws_secrets",
                ),
                (
                    HIGH_CFG,
                    "Secrets Manager rotation required key for secret {n}",
                    "Secret definitions must include a Secrets Manager rotation required key for secret {n}.",
                    "Static secrets eventually leak.",
                    "Rotation Lambda ownership is assigned.",
                    "configuration.aws_secrets",
                ),
                (
                    HIGH_SEC,
                    "plaintext environment secret for Lambda {n}",
                    "Lambdas must not inject a plaintext environment secret for Lambda {n}.",
                    "Environment variables appear in consoles and dumps.",
                    "Fetch secrets at runtime from Secrets Manager/SSM.",
                    "security.aws_secrets",
                ),
                (
                    MED_SEC,
                    "shared secret across environments for service {n}",
                    "Services must not reuse a shared secret across environments for service {n} unless an exception is explicitly documented.",
                    "Prod/non-prod secret sharing collapses blast radius.",
                    "Each environment has unique secrets.",
                    "security.aws_secrets",
                ),
                (
                    LOW_REV,
                    "prefer SSM Parameter Store SecureString for config {n}",
                    "Apps should prefer SSM Parameter Store SecureString for config {n} when values are sensitive.",
                    "String parameters leave values readable.",
                    "KMS CMKs encrypt SecureString params.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented static API key for SaaS vendor {n}",
                    "Integrations may adopt a documented static API key for SaaS vendor {n} when the vendor lacks OAuth.",
                    "Some vendors only offer static keys.",
                    "Keys rotate on a documented cadence.",
                    None,
                ),
                (
                    HIGH_SEC,
                    "secrets in AMI or container layer for image {n}",
                    "Images must not bake secrets in AMI or container layer for image {n}.",
                    "Layer history retains deleted secrets.",
                    "Inject secrets at runtime only.",
                    "security.aws_secrets",
                ),
                (
                    MED_SEC,
                    "unencrypted EBS snapshot share for account {n}",
                    "Operators must not create an unencrypted EBS snapshot share for account {n} unless an exception is explicitly documented.",
                    "Shared snapshots leak disk contents.",
                    "Shares require encryption and approvals.",
                    "security.aws_secrets",
                ),
            ],
        ),
        (
            "s3",
            [
                (
                    HIGH_SEC,
                    "public ACL grant for bucket {n}",
                    "Buckets must not apply a public ACL grant for bucket {n}.",
                    "Public ACLs are a leading cause of data leaks.",
                    "Block Public Access is account-enforced.",
                    "security.aws_s3",
                ),
                (
                    HIGH_CFG,
                    "S3 Block Public Access required key for account {n}",
                    "Accounts must set an S3 Block Public Access required key for account {n}.",
                    "Per-bucket mistakes still happen without account BPA.",
                    "Exceptions are rare and ticketed.",
                    "configuration.aws_s3",
                ),
                (
                    HIGH_SEC,
                    "public-read canned ACL in SDK put for object {n}",
                    "Application code must not use a public-read canned ACL in SDK put for object {n}.",
                    "SDK canned ACLs bypass infrastructure intent.",
                    "Object ownership enforces bucket owner enforced.",
                    "security.aws_s3",
                ),
                (
                    MED_SEC,
                    "bucket policy principal star for bucket {n}",
                    "Policies must not use a bucket policy principal star for bucket {n} unless an exception is explicitly documented.",
                    "Principal * opens anonymous access paths.",
                    "Principals are account or service scoped.",
                    "security.aws_s3",
                ),
                (
                    LOW_REV,
                    "prefer Object Lock for compliance bucket {n}",
                    "Compliance stores should prefer Object Lock for compliance bucket {n} when retention is legally required.",
                    "Soft deletes are insufficient for some regimes.",
                    "Retention modes are legal-reviewed.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented public website bucket for static site {n}",
                    "Web teams may adopt a documented public website bucket for static site {n} when hosting public static assets.",
                    "Static websites are intentionally public.",
                    "CloudFront and OAC are preferred fronts.",
                    None,
                ),
                (
                    HIGH_CFG,
                    "S3 versioning required key for backup bucket {n}",
                    "Backup buckets must include an S3 versioning required key for backup bucket {n}.",
                    "Versioning enables recovery from ransomware deletes.",
                    "Lifecycle policies control cost.",
                    "configuration.aws_s3",
                ),
                (
                    MED_SEC,
                    "cross-account get without condition for bucket {n}",
                    "Shares must not allow a cross-account get without condition for bucket {n} unless an exception is explicitly documented.",
                    "Unconditional cross-account gets overshare.",
                    "Conditions bind org ID or VPC endpoints.",
                    "security.aws_s3",
                ),
            ],
        ),
        (
            "security_groups",
            [
                (
                    HIGH_SEC,
                    "0.0.0.0/0 on port 3389 for security group {n}",
                    "Security groups must not allow 0.0.0.0/0 on port 3389 for security group {n}.",
                    "Open RDP is mass-scanned.",
                    "Use SSM or VPN-sourced CIDRs.",
                    "security.aws_sg",
                ),
                (
                    HIGH_SEC,
                    "unrestricted egress 0.0.0.0/0 for sensitive tier {n}",
                    "Sensitive tiers must not allow unrestricted egress 0.0.0.0/0 for sensitive tier {n}.",
                    "Open egress enables exfiltration and SSRF impact.",
                    "Egress is allow-listed by destination.",
                    "security.aws_sg",
                ),
                (
                    HIGH_CFG,
                    "security group description required key for SG {n}",
                    "Security groups must set a security group description required key for SG {n}.",
                    "Empty descriptions hide ownership and intent.",
                    "Descriptions name system and owner.",
                    "configuration.aws_sg",
                ),
                (
                    MED_SEC,
                    " overlapping world-open rules for alb SG {n}",
                    "ALB groups must not keep overlapping world-open rules for alb SG {n} unless an exception is explicitly documented.",
                    "Duplicate opens complicate audits.",
                    "Only 80/443 from the world when required.",
                    "security.aws_sg",
                ),
                (
                    LOW_REV,
                    "prefer prefix lists for shared CIDR sets {n}",
                    "Network teams should prefer prefix lists for shared CIDR sets {n} when many SGs share sources.",
                    "Copied CIDR lists drift.",
                    "Managed prefix lists are the source of truth.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented temporary world-open rule for migration {n}",
                    "Migrators may adopt a documented temporary world-open rule for migration {n} when cutovers need broad testing.",
                    "Short migrations sometimes need open access.",
                    "Rules expire automatically.",
                    None,
                ),
                (
                    HIGH_ARCH,
                    "database SG attached to public interface for db {n}",
                    "Databases must not use a database SG attached to public interface for db {n}.",
                    "Public interfaces plus DB ports equal exposure.",
                    "DB SGs only accept app-tier SGs.",
                    "architecture.aws_sg",
                ),
                (
                    MED_SEC,
                    "self-referencing SG with all ports for tier {n}",
                    "Tiers must not use a self-referencing SG with all ports for tier {n} unless an exception is explicitly documented.",
                    "All-ports self refs defeat microsegmentation.",
                    "Allow only required ports between peers.",
                    "security.aws_sg",
                ),
            ],
        ),
        (
            "root_mfa",
            [
                (
                    HIGH_CFG,
                    "root MFA required key for account {n}",
                    "Accounts must enforce a root MFA required key for account {n}.",
                    "Unprotected root can destroy the account.",
                    "Root credentials stay vaulted offline.",
                    "configuration.aws_root",
                ),
                (
                    HIGH_SEC,
                    "root access key creation for account {n}",
                    "Operators must not perform a root access key creation for account {n}.",
                    "Root access keys are almost never justified.",
                    "SCPs deny root key creation.",
                    "security.aws_root",
                ),
                (
                    HIGH_SEC,
                    "daily root console usage for account {n}",
                    "Teams must not rely on daily root console usage for account {n}.",
                    "Routine root use expands exposure.",
                    "Delegated admin roles replace root tasks.",
                    "security.aws_root",
                ),
                (
                    MED_SEC,
                    "root email without monitored inbox for account {n}",
                    "Accounts must not keep a root email without monitored inbox for account {n} unless an exception is explicitly documented.",
                    "Unmonitored root email misses recovery and alerts.",
                    "Security-owned aliases receive root mail.",
                    "security.aws_root",
                ),
                (
                    LOW_REV,
                    "prefer hardware MFA for root user {n}",
                    "Security teams should prefer hardware MFA for root user {n} when protecting high-value accounts.",
                    "Virtual MFA can be cloned with the phone.",
                    "Hardware tokens are dual-custodied.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented root login for account closure {n}",
                    "Finance may adopt a documented root login for account closure {n} when AWS requires root for closure flows.",
                    "Some account lifecycle actions need root.",
                    "Logins are witnessed and ticketed.",
                    None,
                ),
                (
                    HIGH_CFG,
                    "organization SCP deny-root-keys setting for ou {n}",
                    "Organizations must apply an organization SCP deny-root-keys setting for ou {n}.",
                    "Account-local discipline fails under pressure.",
                    "SCPs provide a hard guardrail.",
                    "configuration.aws_root",
                ),
                (
                    MED_SEC,
                    "shared root password manager entry for team {n}",
                    "Teams must not keep a shared root password manager entry for team {n} unless an exception is explicitly documented.",
                    "Shared root passwords defeat accountability.",
                    "Break-glass sealed envelopes replace sharing.",
                    "security.aws_root",
                ),
            ],
        ),
        (
            "backups",
            [
                (
                    HIGH_CFG,
                    "AWS Backup plan required key for workload {n}",
                    "Workloads must declare an AWS Backup plan required key for workload {n}.",
                    "Missing backups turn incidents into data loss.",
                    "Plans define frequency and cold storage.",
                    "configuration.aws_backup",
                ),
                (
                    HIGH_CFG,
                    "cross-region backup copy setting for vault {n}",
                    "Critical vaults must enable a cross-region backup copy setting for vault {n}.",
                    "Single-region disasters destroy local backups.",
                    "Copies land in a isolated account.",
                    "configuration.aws_backup",
                ),
                (
                    HIGH_SEC,
                    "backup vault policy principal star for vault {n}",
                    "Vault policies must not use a backup vault policy principal star for vault {n}.",
                    "Open vault policies allow backup destruction.",
                    "Vault access is tightly role-scoped.",
                    "security.aws_backup",
                ),
                (
                    MED_SEC,
                    "disabled backup vault lock for vault {n}",
                    "Compliance vaults must not keep a disabled backup vault lock for vault {n} unless an exception is explicitly documented.",
                    "Unlocked vaults can be mass-deleted.",
                    "Vault Lock in compliance mode for regulated data.",
                    "security.aws_backup",
                ),
                (
                    LOW_REV,
                    "prefer restore-test drills for backup plan {n}",
                    "Ops should prefer restore-test drills for backup plan {n} when validating recoverability.",
                    "Untested backups fail during incidents.",
                    "Drill results are recorded as evidence.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented shorter retention for nonprod vault {n}",
                    "Nonprod may adopt a documented shorter retention for nonprod vault {n} when cost outweighs long retention.",
                    "Nonprod data may not need long retention.",
                    "Prod retention remains stricter.",
                    None,
                ),
                (
                    HIGH_CFG,
                    "RDS automated backup retention setting for database {n}",
                    "Databases must set an RDS automated backup retention setting for database {n}.",
                    "Zero retention removes point-in-time recovery.",
                    "Retention meets RPO commitments.",
                    "configuration.aws_backup",
                ),
                (
                    MED_SEC,
                    "manual snapshot without encryption for database {n}",
                    "Operators must not create a manual snapshot without encryption for database {n} unless an exception is explicitly documented.",
                    "Unencrypted snapshots are easy to exfiltrate.",
                    "Snapshots inherit encryption requirements.",
                    "security.aws_backup",
                ),
            ],
        ),
        (
            "tagging_compute",
            [
                (
                    HIGH_CFG,
                    "mandatory cost allocation tag setting for account {n}",
                    "Accounts must enforce a mandatory cost allocation tag setting for account {n}.",
                    "Untagged resources hide owners and cost.",
                    "SCPs or Config rules enforce keys.",
                    "configuration.aws_tagging",
                ),
                (
                    HIGH_CFG,
                    "instance metadata hop limit setting for ASG {n}",
                    "Compute baselines must set an instance metadata hop limit setting for ASG {n}.",
                    "IMDSv1 without hop limits enables SSRF key theft.",
                    "Require IMDSv2 with hop limit 1 for containers.",
                    "configuration.aws_compute",
                ),
                (
                    HIGH_SEC,
                    "admin IAM instance profile for web ASG {n}",
                    "Web tiers must not attach an admin IAM instance profile for web ASG {n}.",
                    "Overprivileged instance roles are a prime target.",
                    "Roles grant only the API calls needed.",
                    "security.aws_compute",
                ),
                (
                    MED_SEC,
                    "public EC2 with elastic IP for app {n}",
                    "Apps must not place a public EC2 with elastic IP for app {n} unless an exception is explicitly documented.",
                    "Public instances bypass private network controls.",
                    "Prefer private instances behind load balancers.",
                    "security.aws_compute",
                ),
                (
                    LOW_REV,
                    "prefer Graviton or right-sized instance families for service {n}",
                    "Platforms should prefer Graviton or right-sized instance families for service {n} when optimizing cost without weakening isolation.",
                    "Oversized fleets waste spend and expand patch surface.",
                    "Rightsizing reviews run monthly.",
                    None,
                ),
                (
                    LOW_MAY,
                    "documented untagged sandbox resource for experiment {n}",
                    "Builders may adopt a documented untagged sandbox resource for experiment {n} when prototyping for under 72 hours.",
                    "Short experiments sometimes precede tagging.",
                    "Sandbox accounts auto-reap untagged resources.",
                    None,
                ),
                (
                    HIGH_ARCH,
                    "shared SSH keypair across fleet for env {n}",
                    "Fleets must not reuse a shared SSH keypair across fleet for env {n}.",
                    "Shared keys complicate rotation and attribution.",
                    "Prefer SSM Session Manager without SSH keys.",
                    "architecture.aws_compute",
                ),
                (
                    MED_SEC,
                    "user-data containing secrets for launch template {n}",
                    "Launch templates must not include user-data containing secrets for launch template {n} unless an exception is explicitly documented.",
                    "User-data is readable to many operators and APIs.",
                    "Secrets are fetched after boot from a vault.",
                    "security.aws_compute",
                ),
            ],
        ),
    ]

    idx = 0
    for theme, specs in themes:
        for slot, spec in enumerate(specs, start=1):
            band_spec, marker_t, decision_t, context, consequences, pattern_over = spec
            marker = marker_t.format(n=slot) + f" (AWS-{theme}-{slot})"
            decision = decision_t.format(n=slot).replace(marker_t.format(n=slot), marker, 1)
            adr_id = f"ADR-{start + idx}"
            title_core = marker.split(" (")[0]
            out.append(
                _entry(
                    adr_id,
                    f"AWS {theme}: {title_core[:1].upper() + title_core[1:]}",
                    band_spec,
                    marker,
                    decision,
                    context,
                    consequences,
                    aws=[theme],
                    pattern_override=pattern_over,
                )
            )
            idx += 1
    assert len(out) == 80, len(out)
    return out


def build_taxonomy() -> list[dict]:
    entries = build_owasp_entries() + build_aws_entries()
    assert len(entries) == 200, len(entries)
    # Labels pattern_id must match what extract's suggestPattern will emit for harness docs;
    # store extract_pattern separately for labels while keeping fine-grained pattern in frontmatter.
    for e in entries:
        e["extract_pattern"] = extract_pattern_for(e)
    return entries


def extract_pattern_for(entry: dict) -> str:
    """Map to patterns suggestPattern currently (or after our expansion) returns."""
    # Prefer the fine-grained id when it is a known extract target; else family.
    pid = entry["pattern_id"]
    if pid in {
        "configuration.bounds_or_required_values",
        "security.source_pattern",
        "architecture.dependency_or_boundary",
        "manual_review",
    }:
        return pid
    if pid.startswith("configuration."):
        return "configuration.bounds_or_required_values"
    if pid.startswith("security."):
        return "security.source_pattern"
    if pid.startswith("architecture."):
        return "architecture.dependency_or_boundary"
    return "manual_review"


def main() -> int:
    entries = build_taxonomy()
    ADR_DIR.mkdir(parents=True, exist_ok=True)
    for path in ADR_DIR.glob("ADR-*.md"):
        path.unlink()

    expected = []
    index = []
    for e in entries:
        (ADR_DIR / f"{e['id']}.md").write_text(render_madr(e), encoding="utf-8")
        expected.append(
            {
                "adr_id": e["id"],
                "statement_contains": e["marker"],
                "modality": e["modality"],
                "pattern_id": e["extract_pattern"],
                "catalog_pattern_id": e["pattern_id"],
                "confidence_band": e["band"],
                "escalate_expected": e["escalate"],
            }
        )
        index.append(
            {
                "id": e["id"],
                "title": e["title"],
                "owasp": e["owasp"],
                "aws": e["aws"],
                "pattern_id": e["pattern_id"],
                "band": e["band"],
            }
        )

    labels = {
        "expected_candidates": expected,
        "thresholds": {
            "min_precision": 0.55,
            "min_recall": 0.55,
            "max_escalation_rate": 0.85,
        },
        "corpus_size": len(entries),
        "themes": {"owasp_top_10": 120, "aws_security": 80},
    }
    LABELS_PATH.write_text(json.dumps(labels, indent=2) + "\n", encoding="utf-8")
    TAXONOMY_PATH.write_text(dump_taxonomy_yaml(entries), encoding="utf-8")
    INDEX_PATH.write_text(json.dumps({"adrs": index, "corpus_size": len(index)}, indent=2) + "\n", encoding="utf-8")
    print(f"Wrote {len(entries)} MADRs under {ADR_DIR}")
    print(f"Wrote labels to {LABELS_PATH}")
    print(f"Wrote taxonomy to {TAXONOMY_PATH}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
