# Clean counterparts: bounded abuse controls and no dynamic plugin loads.

RATE_LIMIT = 100
LOCKOUT_THRESHOLD = 5


def load_extension():
    import approved_extension

    return approved_extension
