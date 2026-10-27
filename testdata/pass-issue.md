# ADR Assessment fixture: compliant

### File: internal/domain/customer.go

```go
package domain

import "strings"

func NormalizeCustomerName(name string) string {
	return strings.TrimSpace(name)
}
```
