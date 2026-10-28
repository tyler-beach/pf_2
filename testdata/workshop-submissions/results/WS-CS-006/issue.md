# golang-jwt parser fragment [code_snippet]

<!-- submission_kind: code_snippet -->
<!-- provenance:
  source_url: https://github.com/golang-jwt/jwt/blob/main/parser.go
  repo: golang-jwt/jwt
  path: parser.go
  license: MIT
  retrieved: 2026-10-28
  excerpt_sha256_12: d31f04ac04e7
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **code_snippet**. Domains: auth, go, apis.

### File: parser.go

```go
package jwt

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

const tokenDelimiter = "."

type Parser struct {
	// If populated, only these methods will be considered valid.
	validMethods []string

	// Use JSON Number format in JSON decoder.
	useJSONNumber bool

	// Skip claims validation during token parsing.
	skipClaimsValidation bool

	validator *Validator

	decodeStrict bool

	decodePaddingAllowed bool
}

// NewParser creates a new Parser with the specified options
func NewParser(options ...ParserOption) *Parser {
	p := &Parser{
		validator: &Validator{},
	}

	// Loop through our parsing options and apply them
	for _, option := range options {
		option(p)
	}

	return p
}

// Parse parses, validates, verifies the signature and returns the parsed token.
// keyFunc will receive the parsed token and should return the key for validating.
func (p *Parser) Parse(tokenString string, keyFunc Keyfunc) (*Token, error) {
	return p.ParseWithClaims(tokenString, MapClaims{}, keyFunc)
}

// ParseWithClaims parses, validates, and verifies like Parse, but supplies a default object implementing the Claims
// interface. This provides default values which can be overridden and allows a caller to use their own type, rather
// than the default MapClaims implementation of Claims.
//
// Note: If you provide a custom claim implementation that embeds one of the standard claims (such as RegisteredClaims),
// make sure that a) you either embed a non-pointer version of the claims or b) if you are using a pointer, allocate the
```
