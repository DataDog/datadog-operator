// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogclient

import (
	"context"
	"net/http"
)

type ifNoneMatchKey struct{}

// WithIfNoneMatch returns a context that makes GET requests sent by clients from
// this package carry etag in an If-None-Match header. The API client does not
// expose request headers, so the ETag travels in the request context. An empty
// etag returns ctx unchanged.
func WithIfNoneMatch(ctx context.Context, etag string) context.Context {
	if etag == "" {
		return ctx
	}
	return context.WithValue(ctx, ifNoneMatchKey{}, etag)
}

// etagTransport sets If-None-Match on GET requests whose context carries an
// ETag from WithIfNoneMatch.
type etagTransport struct {
	base http.RoundTripper
}

// NewETagTransport wraps base, or http.DefaultTransport if base is nil, so that
// GET requests honor WithIfNoneMatch.
func NewETagTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &etagTransport{base: base}
}

func (t *etagTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Only GETs: on writes, If-None-Match would turn into a precondition.
	if etag, ok := req.Context().Value(ifNoneMatchKey{}).(string); ok && req.Method == http.MethodGet && req.Header.Get("If-None-Match") == "" {
		// A RoundTripper must not modify the caller's request.
		req = req.Clone(req.Context())
		req.Header.Set("If-None-Match", etag)
	}
	return t.base.RoundTrip(req)
}
