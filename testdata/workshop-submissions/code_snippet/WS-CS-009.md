# chi jwtauth middleware [code_snippet]

<!-- submission_kind: code_snippet -->
<!-- provenance:
  source_url: https://github.com/go-chi/jwtauth/blob/master/jwtauth.go
  repo: go-chi/jwtauth
  path: jwtauth.go
  license: MIT
  retrieved: 2026-10-28
  excerpt_sha256_12: 558ed5fd2ee8
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **code_snippet**. Domains: auth, go, apis.

### File: jwtauth.go

```go
package jwtauth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwt"
	"github.com/lestrrat-go/jwx/v3/transform"
)

type JWTAuth struct {
	alg             jwa.SignatureAlgorithm
	signKey         interface{} // private-key
	verifyKey       interface{} // public-key, only used by RSA and ECDSA algorithms
	verifier        jwt.ParseOption
	validateOptions []jwt.ValidateOption
}

var (
	TokenCtxKey = &contextKey{"Token"}
	ErrorCtxKey = &contextKey{"Error"}
)

var (
	ErrUnauthorized = errors.New("token is unauthorized")
	ErrExpired      = errors.New("token is expired")
	ErrNBFInvalid   = errors.New("token nbf validation failed")
	ErrIATInvalid   = errors.New("token iat validation failed")
	ErrNoTokenFound = errors.New("no token found")
	ErrAlgoInvalid  = errors.New("algorithm mismatch")
)

func New(alg string, signKey interface{}, verifyKey interface{}, validateOptions ...jwt.ValidateOption) *JWTAuth {
	sigAlg, _ := jwa.LookupSignatureAlgorithm(alg)
	ja := &JWTAuth{
		alg:             sigAlg,
		signKey:         signKey,
		verifyKey:       verifyKey,
		validateOptions: validateOptions,
	}

	if ja.verifyKey != nil {
		ja.verifier = jwt.WithKey(ja.alg, ja.verifyKey)
	} else {
		ja.verifier = jwt.WithKey(ja.alg, ja.signKey)
	}

	return ja
}

// Verifier http middleware handler will verify a JWT string from a http request.
```
