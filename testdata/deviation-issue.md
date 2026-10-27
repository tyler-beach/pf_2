# ADR Assessment fixture: deviations

### File: internal/domain/customer.go

```go
package domain

import (
	"internal/infrastructure/db"
	"net/http"
)

func LoadCustomer() {
	_ = db.Connection{}
	_ = http.MethodGet
}
```
