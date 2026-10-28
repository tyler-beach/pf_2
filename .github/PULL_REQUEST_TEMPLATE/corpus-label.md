<!-- Use title marker: [corpus-label] -->

## Corpus labeling PR

Architects submit ADR Markdown + expected labels here (not chat).

### Checklist

- [ ] Prefer extending `scripts/generate_security_corpus.py` / regenerating via that script for bulk OWASP/AWS adds
- [ ] Added/updated MADR Markdown under `testdata/corpus/adrs/` (Decision holds the only normative language)
- [ ] Updated `testdata/corpus/labels.json` + `taxonomy.yml` (`statement_contains`, `modality`, `pattern_id`, `confidence_band`, `escalate_expected`)
- [ ] Ran `corpus-eval` workflow or local harness against the new labels
- [ ] No secrets in ADR bodies

### Notes

```
[corpus-label]
```
