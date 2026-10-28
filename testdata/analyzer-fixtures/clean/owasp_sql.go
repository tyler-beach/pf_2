package clean

func SafeQuery() string {
	return "SELECT * FROM users WHERE name=?"
}
