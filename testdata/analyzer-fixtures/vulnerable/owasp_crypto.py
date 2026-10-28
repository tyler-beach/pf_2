import hashlib
import os
import requests
import urllib.request

hashlib.md5(b"x")
hashlib.sha1(b"x")
requests.get(input("url: "), verify=False)
urllib.request.urlopen(input("url: "))
password = "SuperSecretPassword1"
AWS_KEY = "AKIAIOSFODNN7EXAMPLE"
pem = "-----BEGIN RSA PRIVATE KEY-----\nMIIE\n-----END RSA PRIVATE KEY-----"
# Intentionally omit live Slack-shaped tokens; GitHub push protection flags them.
# Cover slack-like secrets with a non-exfiltrable test marker instead:
slack = "pf-test-slack-token-0000000000"
os.environ["AWS_SECRET_ACCESS_KEY"] = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
