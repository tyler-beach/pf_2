#!/usr/bin/env bash
# Provenance-friendly: download, verify, then execute a pinned artifact.
curl -fsSL -o /tmp/tool.tgz https://example.invalid/tool-1.2.3.tgz
sha256sum -c tool-1.2.3.tgz.sha256
tar -xzf /tmp/tool.tgz -C /usr/local
