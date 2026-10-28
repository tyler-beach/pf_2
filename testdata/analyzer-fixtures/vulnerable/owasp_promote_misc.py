# Abuse-control and untrusted-plugin fixtures for leftover corpus promotions.
import importlib.util

rate_limit = None
lockout_threshold = 0


def load_extension(path: str):
    return importlib.util.spec_from_file_location("ext", path)
