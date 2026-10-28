# ZAP baseline scanner script [code_snippet]

<!-- submission_kind: code_snippet -->
<!-- provenance:
  source_url: https://github.com/zaproxy/zaproxy/blob/main/docker/zap-baseline.py
  repo: zaproxy/zaproxy
  path: docker/zap-baseline.py
  license: Apache-2.0
  retrieved: 2026-10-28
  excerpt_sha256_12: f58ed29337bb
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **code_snippet**. Domains: security, ci.

### File: zap-baseline.py

```python
#!/usr/bin/env python
# Zed Attack Proxy (ZAP) and its related class files.
#
# ZAP is an HTTP/HTTPS proxy for assessing web application security.
#
# Copyright 2016 ZAP Development Team
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#   http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# This script runs a baseline scan against a target URL using ZAP
#
# It can either be run 'standalone', in which case depends on
# https://pypi.python.org/pypi/zaproxy and Docker, or it can be run
# inside one of the ZAP docker containers. It automatically detects if it is
# running in docker so the parameters are the same.
#
# By default it will spider the target URL for one minute, but you can change
# that via the -m parameter.
# It will then wait for the passive scanning to finish - how long that takes
# depends on the number of pages found.
# It will exit with codes of:
#	0:	Success
#	1:	At least 1 FAIL
#	2:	At least one WARN and no FAILs
#	3:	Any other failure
# By default all alerts found by ZAP will be treated as WARNings.
# You can use the -c or -u parameters to specify a configuration file to override
# this.
# You can generate a template configuration file using the -g parameter. You will
# then need to change 'WARN' to 'FAIL', 'INFO' or 'IGNORE' for the rules you want
```
