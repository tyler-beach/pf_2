# Language support and coverage

Pathfinder has two distinct input surfaces:

1. **Repository assessment** runs a pinned OpenGrep version over the repository snapshot and evaluates normalized, source-located findings against accepted requirement bindings. The language table below describes rule families exercised by CI.
2. **Issue-submitted snippets** remain a constrained assessment surface. Only Go and JSON currently have direct deterministic snippet verifiers. Other languages are preserved in the result as unassessed; the issue workflow does not execute submitted code or run external analyzers on issue-provided snippets. Missing or unsupported evidence must remain indeterminate rather than pass.

OpenGrep supports a wider language set than Pathfinder currently has security rules for. Adding parser support alone does not constitute a meaningful policy check, so the following rules each have a vulnerable fixture and a clean fixture that run in CI with the pinned OpenGrep release.

## CI-validated rule families

| Language | Fixture extensions | Rule ID | Current check |
|---|---|---|---|
| Go | `.go` | `pathfinder.go-shell-command` | Shell invocation through `sh -c` |
| Python | `.py` | `pathfinder.python-dynamic-eval` | Dynamic source evaluation |
| JavaScript | `.js` | `pathfinder.no-dynamic-eval` | Dynamic source evaluation |
| TypeScript | `.ts` | `pathfinder.no-dynamic-eval` | Dynamic source evaluation |
| Java | `.java` | `pathfinder.java-runtime-exec` | Runtime process execution |
| C# | `.cs` | `pathfinder.csharp-process-start` | Process creation |
| PHP | `.php` | `pathfinder.no-dynamic-eval` | Dynamic source evaluation |
| Ruby | `.rb` | `pathfinder.ruby-dynamic-eval` | Dynamic source evaluation |
| Bash | `.bash` | `pathfinder.bash-dynamic-eval` | Shell `eval` |
| C | `.c` | `pathfinder.c-shell-command` | `system` invocation |
| C++ | `.cpp` | `pathfinder.cpp-shell-command` | `std::system` invocation |
| Rust | `.rs` | `pathfinder.rust-shell-command` | `sh -c` process launch |
| Kotlin | `.kt` | `pathfinder.kotlin-runtime-exec` | Runtime process execution |
| Scala | `.scala` | `pathfinder.scala-runtime-exec` | Runtime process execution |
| Elixir | `.ex` | `pathfinder.elixir-dynamic-eval` | Dynamic source evaluation |
| Dart | `.dart` | `pathfinder.dart-process-run` | Shell process and `-c` arguments |
| Swift | `.swift` | `pathfinder.swift-shell-command` | Shell-style process arguments |
| Lua | `.lua` | `pathfinder.lua-dynamic-eval` | Dynamic source compilation |
| Visual Basic | `.vb` | `pathfinder.visualbasic-process-start` | Process creation |
| OCaml | `.ml` | `pathfinder.ocaml-system-command` | `Sys.command` invocation |

## Catalog bindings

Each CI-validated rule family above is bound to an accepted `analyzer-rule` requirement in `adr/requirements.json` (ADR-003 through ADR-006) with explicit `source_path_prefixes` and `source_extensions`. Reusable pattern IDs for those bindings are defined in `adr/patterns.json`.

## Test and enforcement boundaries

- The CI fixture gate requires a complete scan and exact expected rule-family set for the vulnerable fixture corpus; a clean corpus must produce zero findings. Any rule/parser behavior change is therefore visible as a regression.
- Repository scan coverage is limited to enabled rules and the paths/languages actually covered by the analyzer. A clean scan means only that those checks found nothing in the scanned scope.
- A rule match is a review signal, not proof that input is attacker-controlled or that exploitability is confirmed. Rules should be reviewed for false-positive behavior before a matching obligation is made blocking.
- New language rules should arrive with both a vulnerable fixture and a clean fixture. Pin the analyzer version and binary digest in CI, preserve raw SARIF, and require the full suite to pass before changing enforcement bindings.
- The underlying OpenGrep engine advertises support for 30+ languages; Pathfinder's enabled rule families are intentionally narrower than the engine's parser list. See the [official OpenGrep repository](https://github.com/opengrep/opengrep) for its current language inventory.
