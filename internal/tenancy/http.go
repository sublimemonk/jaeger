// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tenancy

import (
	"context"
	"net/http"

	"google.golang.org/grpc/metadata"
)

// PropagationHandler returns a http.Handler containing the logic to extract
// the tenancy header of the http.Request and insert the tenant into request.Context
// for propagation. The token can be accessed via tenancy.GetTenant().
func ExtractTenantHTTPHandler(tc *Manager, h http.Handler) http.Handler {
	if !tc.Enabled {
		return h
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenants := r.Header.Values(tc.Header)
		// A request with more than one tenant header is ambiguous, so it is rejected
		// instead of being served as the first one, as the gRPC path does.
		if len(tenants) > 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("extra tenant header"))
			return
		}
		tenant := ""
		if len(tenants) == 1 {
			tenant = tenants[0]
		}
		if tenant == "" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("missing tenant header"))
			return
		}

		if !tc.Valid(tenant) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("unknown tenant"))
			return
		}

		ctx := WithTenant(r.Context(), tenant)
		h.ServeHTTP(w, r.WithContext(ctx))
	})
}

// MetadataAnnotator returns a function suitable for propagating tenancy
// via github.com/grpc-ecosystem/grpc-gateway/runtime.NewServeMux
func (tc *Manager) MetadataAnnotator() func(context.Context, *http.Request) metadata.MD {
	return func(_ context.Context, req *http.Request) metadata.MD {
		tenants := req.Header.Values(tc.Header)
		if len(tenants) == 0 || (len(tenants) == 1 && tenants[0] == "") {
			// The HTTP request lacked the tenancy header.  Pass along
			// empty metadata -- the gRPC query service will reject later.
			return metadata.Pairs()
		}
		// Forward every value, so that the gRPC query service rejects
		// a request with more than one tenant header.
		md := metadata.MD{}
		md.Append(tc.Header, tenants...)
		return md
	}
}
