# Prometheus storage architecture [tech_spec]

<!-- submission_kind: tech_spec -->
<!-- provenance:
  source_url: https://github.com/prometheus/prometheus/blob/main/docs/storage.md
  repo: prometheus/prometheus
  path: docs/storage.md
  license: Apache-2.0
  retrieved: 2026-10-28
  excerpt_sha256_12: 3ba315e1df33
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **tech_spec**. Domains: reliability, data, observability.

## Technical specification / ADR (excerpt)

---
title: Storage
sort_rank: 5
---

Prometheus includes a local on-disk time series database, but also optionally integrates with remote storage systems.

## Local storage

Prometheus's local time series database stores data in a custom, highly efficient format on local storage.

### On-disk layout

Ingested samples are grouped into two-hour blocks. Each block consists of a directory that 
contains a chunks subdirectory with all the time series samples for that time window, 
a metadata file,  and an index file (which maps metric names and labels to the time series 
in the chunks directory). The samples in the chunks directory are organized into one 
or more segment files, each up to 512 MB by default. When series are deleted via the API, 
the deletion records are stored in separate tombstone files rather than being immediately 
removed from the chunk segments.

The current block for incoming samples is kept in memory and is not fully
persisted. It is secured against crashes by a write-ahead log (WAL) that can be
replayed when the Prometheus server restarts. Write-ahead log files are stored
in the `wal` directory in 128MB segments. These files contain raw data that
has not yet been compacted; thus they are significantly larger than regular block
files. Prometheus will retain a minimum of three write-ahead log files.
High-traffic servers may retain more than three WAL files in order to keep at
least two hours of raw data.

A Prometheus server's data directory looks something like this:

```
./data
├── 01BKGV7JBM69T2G1BGBGM6KB12
│   └── meta.json
├── 01BKGTZQ1SYQJTR4PB43C8PD98
│   ├── chunks
│   │   └── 000001
│   ├── tombstones
│   ├── index
│   └── meta.json
├── 01BKGTZQ1HHWHV8FBJXW1Y3W0K
│   └── meta.json
├── 01BKGV7JC0RY8A6MACW02A2PJD
│   ├── chunks
│   │   └── 000001
│   ├── tombstones
│   ├── index
│   └── meta.json
├── chunks_head
│   └── 000001
└── wal
    ├── 000000002
    └── checkpoint.00000001

