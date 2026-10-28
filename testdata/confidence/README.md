# Statistical confidence artifacts

| File | Role |
|---|---|
| `gold-extract-v1.json` | Seed gold labels for extract quality (`extract_correct`, `expected_band`, assess/assist hooks). |
| `calibration-v1.json` | Fitted Platt + conformal params (mirror of embedded `internal/obligations/calibration/params-v1.json`). |

## Methods

- **Heuristic** (`confidence_method=heuristic`): deterministic triage score from modality / pattern / ambiguity — **not** a probability.
- **Platt / logistic** (`confidence_p`): calibrated P(extract_correct) from a small feature vector (heuristic score is one *input* feature).
- **Inductive conformal bands** (`confidence_band`, `confidence_coverage`): prediction sets at target coverage (default 90%) under an **exchangeability** assumption with the gold calibration set. Singleton `{high}` → high; otherwise widen.

Refit after growing gold (including the 50-submissions pack):

```bash
go test ./internal/obligations/calibration/ -run TestRefitParamsCommitted -count=1
```

Do not claim calibrated meaning when method falls back to heuristic.
