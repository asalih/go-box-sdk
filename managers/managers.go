// Package managers contains the focused set of Box API endpoint managers used
// by the evidence-repository use case: authorization, files, folders, uploads,
// chunked uploads, and downloads. It mirrors the corresponding files under
// src/managers in the Box SDK.
//
// Each manager carries an optional Authentication and a NetworkSession, and
// issues requests through the session's network client. The async Promise<T>
// signatures of the source map to (T, error); the cancellation token maps to
// context.Context threaded as the first argument.
package managers

import (
	"context"
	"strings"

	"github.com/asalih/go-box-sdk/networking"
)

// baseManager holds the auth and session shared by every manager. It is
// embedded by each concrete manager to mirror the common fields on the source
// manager classes.
type baseManager struct {
	Auth           networking.Authentication
	NetworkSession *networking.NetworkSession
}

// fetch dispatches a request, wiring the manager's auth and session into the
// options exactly as the source managers do before calling networkClient.fetch.
func (m baseManager) fetch(ctx context.Context, options *networking.FetchOptions) (*networking.FetchResponse, error) {
	options.Auth = m.Auth
	options.NetworkSession = m.NetworkSession
	session := m.NetworkSession
	if session == nil {
		session = networking.NewNetworkSession()
		options.NetworkSession = session
	}
	client := session.NetworkClient
	if client == nil {
		client = networking.NewBoxNetworkClient()
	}
	return client.Fetch(ctx, options)
}

// prepareParams drops entries with empty values, mirroring the source
// prepareParams which filters out undefined query/header values.
func prepareParams(params map[string]string) map[string]string {
	out := make(map[string]string, len(params))
	for k, v := range params {
		if v == "" {
			continue
		}
		out[k] = v
	}
	return out
}

// mergeExtraHeaders overlays caller-supplied extra headers onto a base header
// map, mirroring the spread of extraHeaders in the source.
func mergeExtraHeaders(base, extra map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range extra {
		merged[k] = v
	}
	return prepareParams(merged)
}

// joinFields renders the comma-separated "fields" query parameter. It returns
// an empty string when no fields are requested so prepareParams drops it.
func joinFields(fields []string) string {
	return strings.Join(fields, ",")
}
