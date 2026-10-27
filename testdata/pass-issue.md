# ADR Assessment fixture: compliant

### File: internal/domain/customer.go

```go
package domain

import "strings"

func NormalizeCustomerName(name string) string {
	return strings.TrimSpace(name)
}
```

### File: config/service.json

```json
{
  "timeout_seconds": 30,
  "retry_limit": 3
}
```
