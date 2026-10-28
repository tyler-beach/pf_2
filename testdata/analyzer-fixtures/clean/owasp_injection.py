def safe_sql(cursor, user):
    cursor.execute("SELECT * FROM users WHERE name=?", (user,))

def normalize(value: str) -> str:
    return value.strip()
