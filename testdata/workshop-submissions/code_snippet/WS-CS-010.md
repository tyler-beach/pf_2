# Falco default rules excerpt [code_snippet]

<!-- submission_kind: code_snippet -->
<!-- provenance:
  source_url: https://github.com/falcosecurity/rules/blob/main/rules/falco_rules.yaml
  repo: falcosecurity/rules
  path: rules/falco_rules.yaml
  license: Apache-2.0
  retrieved: 2026-10-28
  excerpt_sha256_12: 723c8e8bea89
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **code_snippet**. Domains: security, logging, runtime.

### File: falco_rules.yaml

```yaml
# SPDX-License-Identifier: Apache-2.0
#
# Copyright (C) 2025 The Falco Authors.
#
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

# Information about rules tags and fields can be found here: https://falco.org/docs/rules/#tags-for-current-falco-ruleset
# The initial item in the `tags` fields reflects the maturity level of the rules introduced upon the proposal https://github.com/falcosecurity/rules/blob/main/proposals/20230605-rules-adoption-management-maturity-framework.md
# `tags` fields also include information about the type of workload inspection (host and/or container), and Mitre Attack killchain phases and Mitre TTP code(s)
# Mitre Attack References:
# [1] https://attack.mitre.org/tactics/enterprise/
# [2] https://raw.githubusercontent.com/mitre/cti/master/enterprise-attack/enterprise-attack.json

# Starting with version 8, the Falco engine supports exceptions.
# However the Falco rules file does not use them by default.
- required_engine_version: 0.57.0

- required_plugin_versions:
    - name: container
      version: 0.4.0

# Currently disabled as read/write are ignored syscalls. The nearly
# similar open_write/open_read check for files being opened for
# reading/writing.
# - macro: write
#   condition: (syscall.type=write and fd.type in (file, directory))
# - macro: read
#   condition: (syscall.type=read and fd.type in (file, directory))
```
