// Package grpcplugin connects plugin protocol v3 services to go-plugin.
//
// When NewClientV3 receives a token exchanger, it delegates the caller using
// AuthInfo from the request context. It prefers a signed access token, preserving
// an existing actor chain, and otherwise exchanges the caller's signed ID token.
// The result is sent in x-access-token; no separate ID token is needed because
// the exchanged access token carries the identity and delegated permissions.
// Missing caller credentials fail locally instead of using service credentials,
// except for the host's own service identity (ClientV3Options.IsServiceIdentity),
// which gets the host's service token. Tokens are only minted for the API groups
// the plugin serves (ClientV3Options.Groups), and are scoped to the request's
// namespace when it has a single one.
//
// ServeOpts.Authenticator must verify access-token signatures and the plugin's
// allowed API-group audiences (for example, with authn.NewAccessTokenAuthenticator).
// Without one, servers reject every request unless ServeOpts.InsecureSkipAuthentication
// is set, for local development only: tokens are then parsed without verifying
// them, so handlers get whatever identity a token claims, and the audience and
// namespace checks below are skipped. Servers require exactly one access token and reject a separate ID token
// to prevent conflicting identities. They also check the request's API group
// against the verified audiences, and the token namespace against the route
// namespace and against metadata.namespace of every admission and conversion
// object. Cluster-scoped routes and objects require a wildcard namespace in the token.
// Successful handlers receive AuthInfo including the verified access token for
// onward delegation. Authentication does not replace resource authorization:
// handlers must enforce the caller's permissions for the requested operation
// and validate the kinds inside admission/conversion object payloads.
//
// Admission and route audiences come from the request's resource API group.
// Conversion derives the audience from object GVKs, which must have the same
// non-empty group; the conversion envelope's API group is not the audience.
// A nil client exchanger sends no credentials.
//
// Experimental: Plugin protocol v3 is a work in progress. Its APIs and wire
// format may change or be removed without notice. Use it only for
// experimentation.
package grpcplugin
