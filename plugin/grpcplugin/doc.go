// Package grpcplugin connects plugin protocol v3 services to go-plugin.
//
// When NewClientV3 receives a token exchanger, it delegates the caller using
// AuthInfo from the request context. It prefers a signed access token, preserving
// an existing actor chain, and otherwise exchanges the caller's signed ID token.
// The result is sent in x-access-token; no separate ID token is needed because
// the exchanged access token carries the identity and delegated permissions.
// Missing caller credentials fail locally instead of using service credentials.
//
// ServeOpts.Authenticator must verify access-token signatures and the plugin's
// allowed API-group audiences (for example, with authn.NewAccessTokenAuthenticator).
// Servers require exactly one access token and reject a separate ID token to
// prevent conflicting identities. They also check the request's API group against
// the verified audiences and route namespaces against the token namespace.
// Cluster-scoped routes require a wildcard namespace in the token.
// Successful handlers receive AuthInfo including the verified access token for
// onward delegation. Authentication does not replace resource authorization:
// handlers must enforce the caller's permissions for the requested operation
// and validate namespaces and kinds inside admission/conversion object payloads.
//
// Admission and route audiences come from the request's resource API group.
// Conversion derives the audience from object GVKs, which must have the same
// non-empty group; the conversion envelope's API group is not the audience.
// A nil client exchanger or server authenticator disables its respective step.
//
// Experimental: Plugin protocol v3 is a work in progress. Its APIs and wire
// format may change or be removed without notice. Use it only for
// experimentation.
package grpcplugin
