# Django REST framework authentication [code_snippet]

<!-- submission_kind: code_snippet -->
<!-- provenance:
  source_url: https://github.com/encode/django-rest-framework/blob/master/rest_framework/authentication.py
  repo: encode/django-rest-framework
  path: rest_framework/authentication.py
  license: BSD-3-Clause
  retrieved: 2026-10-28
  excerpt_sha256_12: 135824bd3fe5
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **code_snippet**. Domains: auth, python, apis.

### File: authentication.py

```python
"""
Provides various authentication policies.
"""
import base64
import binascii

from django.contrib.auth import authenticate, get_user_model
from django.middleware.csrf import CsrfViewMiddleware
from django.utils.translation import gettext_lazy as _

from rest_framework import HTTP_HEADER_ENCODING, exceptions


def get_authorization_header(request):
    """
    Return request's 'Authorization:' header, as a bytestring.

    Hide some test client ickyness where the header can be unicode.
    """
    auth = request.headers.get('authorization', b'')
    if isinstance(auth, str):
        # Work around django test client oddness
        auth = auth.encode(HTTP_HEADER_ENCODING)
    return auth


class CSRFCheck(CsrfViewMiddleware):
    def _reject(self, request, reason):
        # Return the failure reason instead of an HttpResponse
        return reason


class BaseAuthentication:
    """
    All authentication classes should extend BaseAuthentication.
    """

    def authenticate(self, request):
        """
        Authenticate the request and return a two-tuple of (user, token).
        """
        raise NotImplementedError(".authenticate() must be overridden.")

    def authenticate_header(self, request):
        """
        Return a string to be used as the value of the `WWW-Authenticate`
        header in a `401 Unauthenticated` response, or `None` if the
        authentication scheme should return `403 Permission Denied` responses.
        """
        pass


class BasicAuthentication(BaseAuthentication):
    """
    HTTP Basic authentication against username/password.
```
