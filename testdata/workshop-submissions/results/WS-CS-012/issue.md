# golang.org/x/oauth2 client fragment [code_snippet]

<!-- submission_kind: code_snippet -->
<!-- provenance:
  source_url: https://github.com/golang/oauth2/blob/master/oauth2.go
  repo: golang/oauth2
  path: oauth2.go
  license: BSD-3-Clause
  retrieved: 2026-10-28
  excerpt_sha256_12: 728c0e6d3f64
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **code_snippet**. Domains: auth, go, apis.

### File: oauth2.go

```go
// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package oauth2 provides support for making
// OAuth2 authorized and authenticated HTTP requests,
// as specified in RFC 6749.
// It can additionally grant authorization with Bearer JWT.
package oauth2 // import "golang.org/x/oauth2"

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2/internal"
)

// NoContext is the default context you should supply if not using
// your own [context.Context].
//
// Deprecated: Use [context.Background] or [context.TODO] instead.
var NoContext = context.TODO()

// RegisterBrokenAuthHeaderProvider previously did something. It is now a no-op.
//
// Deprecated: this function no longer does anything. Caller code that
// wants to avoid potential extra HTTP requests made during
// auto-probing of the provider's auth style should set
// Endpoint.AuthStyle.
func RegisterBrokenAuthHeaderProvider(tokenURL string) {}

// Config describes a typical 3-legged OAuth2 flow, with both the
// client application information and the server's endpoint URLs.
// For the client credentials 2-legged OAuth2 flow, see the
// [golang.org/x/oauth2/clientcredentials] package.
type Config struct {
	// ClientID is the application's ID.
	ClientID string

	// ClientSecret is the application's secret.
	ClientSecret string

	// Endpoint contains the authorization server's token endpoint
	// URLs. These are constants specific to each server and are
	// often available via site-specific packages, such as
	// google.Endpoint or github.Endpoint.
	Endpoint Endpoint

	// RedirectURL is the URL to redirect users going through
	// the OAuth flow, after the resource owner's URLs.
```
