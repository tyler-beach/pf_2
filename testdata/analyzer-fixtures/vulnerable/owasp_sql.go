package vulnerable

import "fmt"

func BadQuery(user string) string {
	return fmt.Sprintf("SELECT * FROM users WHERE name=%s", user)
}
