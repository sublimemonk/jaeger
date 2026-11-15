// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tenancy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

type testHttpHandler struct {
	reached bool
}

func (thh *testHttpHandler) ServeHTTP(http.ResponseWriter, *http.Request) {
	thh.reached = true
}

func TestProgationHandler(t *testing.T) {
	tests := []struct {
		name           string
		tenancyMgr     *Manager
		shouldReach    bool
		requestHeaders map[string][]string
	}{
		{
			name:           "untenanted",
			tenancyMgr:     NewManager(&Options{}),
			requestHeaders: map[string][]string{},
			shouldReach:    true,
		},
		{
			name:           "missing tenant header",
			tenancyMgr:     NewManager(&Options{Enabled: true}),
			requestHeaders: map[string][]string{},
			shouldReach:    false,
		},
		{
			name:           "valid tenant header",
			tenancyMgr:     NewManager(&Options{Enabled: true}),
			requestHeaders: map[string][]string{"x-tenant": {"acme"}},
			shouldReach:    true,
		},
		{
			name:           "unauthorized tenant",
			tenancyMgr:     NewManager(&Options{Enabled: true, Tenants: []string{"megacorp"}}),
			requestHeaders: map[string][]string{"x-tenant": {"acme"}},
			shouldReach:    false,
		},
		{
			name:           "empty tenant header",
			tenancyMgr:     NewManager(&Options{Enabled: true}),
			requestHeaders: map[string][]string{"x-tenant": {""}},
			shouldReach:    false,
		},
		{
			name:           "multiple tenant headers",
			tenancyMgr:     NewManager(&Options{Enabled: true, Tenants: []string{"acme", "megacorp"}}),
			requestHeaders: map[string][]string{"x-tenant": {"acme", "megacorp"}},
			shouldReach:    false,
		},
		{
			name:           "multiple tenant headers without tenant list",
			tenancyMgr:     NewManager(&Options{Enabled: true}),
			requestHeaders: map[string][]string{"x-tenant": {"acme", "megacorp"}},
			shouldReach:    false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := &testHttpHandler{}
			propH := ExtractTenantHTTPHandler(test.tenancyMgr, handler)
			req, err := http.NewRequest(http.MethodGet, "/", strings.NewReader(""))
			for k, vs := range test.requestHeaders {
				for _, v := range vs {
					req.Header.Add(k, v)
				}
			}
			require.NoError(t, err)
			writer := httptest.NewRecorder()
			propH.ServeHTTP(writer, req)
			assert.Equal(t, test.shouldReach, handler.reached)
		})
	}
}

func TestPropagationHandlerRejectsMultipleTenants(t *testing.T) {
	handler := &testHttpHandler{}
	propH := ExtractTenantHTTPHandler(NewManager(&Options{Enabled: true, Tenants: []string{"acme", "megacorp"}}), handler)
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Add("x-tenant", "acme")
	req.Header.Add("x-tenant", "megacorp")
	writer := httptest.NewRecorder()

	propH.ServeHTTP(writer, req)

	assert.False(t, handler.reached)
	assert.Equal(t, http.StatusUnauthorized, writer.Code)
	assert.Equal(t, "extra tenant header", writer.Body.String())
}

func TestMetadataAnnotator(t *testing.T) {
	tests := []struct {
		name           string
		tenancyMgr     *Manager
		requestHeaders map[string][]string
		expected       metadata.MD
	}{
		{
			name:           "missing tenant",
			tenancyMgr:     NewManager(&Options{Enabled: true}),
			requestHeaders: map[string][]string{},
			expected:       metadata.MD{},
		},
		{
			name:           "empty tenant",
			tenancyMgr:     NewManager(&Options{Enabled: true}),
			requestHeaders: map[string][]string{"x-tenant": {""}},
			expected:       metadata.MD{},
		},
		{
			name:           "tenanted",
			tenancyMgr:     NewManager(&Options{Enabled: true}),
			requestHeaders: map[string][]string{"x-tenant": {"acme"}},
			expected:       metadata.MD{"x-tenant": {"acme"}},
		},
		{
			name:           "multiple tenants are all forwarded",
			tenancyMgr:     NewManager(&Options{Enabled: true}),
			requestHeaders: map[string][]string{"x-tenant": {"acme", "megacorp"}},
			expected:       metadata.MD{"x-tenant": {"acme", "megacorp"}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "/", strings.NewReader(""))
			for k, vs := range test.requestHeaders {
				for _, v := range vs {
					req.Header.Add(k, v)
				}
			}
			require.NoError(t, err)
			annotator := test.tenancyMgr.MetadataAnnotator()
			md := annotator(context.Background(), req)
			assert.Equal(t, test.expected, md)
		})
	}
}
