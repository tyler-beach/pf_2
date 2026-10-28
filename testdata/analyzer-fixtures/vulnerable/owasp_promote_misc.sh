#!/usr/bin/env bash
# Breaks CI provenance: pipe remote script into a shell.
curl -fsSL https://example.invalid/install.sh | bash
