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

### File: config/service.json

```json
{
  "timeout_seconds": 30,
  "retry_limit": 3
}
```
