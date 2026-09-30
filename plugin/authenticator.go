package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"

	"github.com/go-jose/go-jose/v4"
	"github.com/grafana/authlib/authn"

	"github.com/grafana/grafana-app-sdk/app"
)

// EnvVarGrafanaJWKSURL names the environment variable holding the
// URL of the JWKS used to verify access tokens. Keys are downloaded on demand.
const EnvVarGrafanaJWKSURL = "GRAFANA_JWKS_URL"

// EnvVarGrafanaJWKS names the environment variable holding the
// JWKS used to verify access tokens, as a JSON document.
const EnvVarGrafanaJWKS = "GRAFANA_JWKS"

// ErrNoSigningKeys is returned by buildAuthenticator when neither
// EnvVarGrafanaJWKSURL nor EnvVarGrafanaJWKS is set.
var ErrNoSigningKeys = errors.New("no signing keys provided: set " + EnvVarGrafanaJWKSURL + " or " + EnvVarGrafanaJWKS)

// buildAuthenticator builds the authenticator for plugin protocol v3 requests
// (see grpcplugin.ServeOpts.Authenticator) from environment variables.
//
// Picks up the signing keys from exactly one of:
// - GRAFANA_JWKS_URL, the JWKS endpoint of the service that signs access tokens
// - GRAFANA_JWKS, the JWKS itself, as a JSON document
//
// Access tokens must be signed by one of those keys and have the plugin ID or
// the manifest's API group as an audience. The host requests the API group
// when it exchanges the caller's token. The URL must use https, except on a loopback host for local
// development, as the keys decide which tokens the plugin trusts. An explicit
// JWKS must contain only public signing keys, each with a key ID.
func buildAuthenticator(pluginID string, manifestData *app.ManifestData) (authn.Authenticator, error) {
	keysURL := os.Getenv(EnvVarGrafanaJWKSURL)
	keysJSON := os.Getenv(EnvVarGrafanaJWKS)
	if keysURL == "" && keysJSON == "" {
		return nil, ErrNoSigningKeys
	}
	// Two sources of trusted keys would be ambiguous.
	if keysURL != "" && keysJSON != "" {
		return nil, fmt.Errorf("set only one of %s and %s", EnvVarGrafanaJWKSURL, EnvVarGrafanaJWKS)
	}

	var audiences []string
	if pluginID != "" {
		audiences = append(audiences, pluginID)
	}
	if manifestData != nil && manifestData.Group != "" {
		audiences = append(audiences, manifestData.Group)
	}
	// An empty audience list would make the verifier skip the audience check.
	if len(audiences) == 0 {
		return nil, errors.New("a plugin ID or manifest API group is required as the access token audience")
	}

	var keys authn.KeyRetriever
	if keysURL != "" {
		if err := validateSigningKeysURL(keysURL); err != nil {
			return nil, fmt.Errorf("invalid %s: %w", EnvVarGrafanaJWKSURL, err)
		}
		keys = authn.NewKeyRetriever(authn.KeyRetrieverConfig{SigningKeysURL: keysURL})
	} else {
		static, err := parseSigningKeys(keysJSON)
		if err != nil {
			return nil, fmt.Errorf("invalid %s: %w", EnvVarGrafanaJWKS, err)
		}
		keys = static
	}

	verifier := authn.NewAccessTokenVerifier(authn.VerifierConfig{AllowedAudiences: audiences}, keys)
	return authn.NewAccessTokenAuthenticator(verifier), nil
}

func validateSigningKeysURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Host == "" {
		return errors.New("an absolute URL is required")
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if host := u.Hostname(); host == "localhost" || net.ParseIP(host).IsLoopback() {
			return nil
		}
		return errors.New("https is required for non-loopback hosts")
	default:
		return fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
}

// staticKeys is a KeyRetriever for a fixed JWKS, indexed by key ID.
type staticKeys map[string]*jose.JSONWebKey

func (k staticKeys) Get(_ context.Context, keyID string) (*jose.JSONWebKey, error) {
	if key, ok := k[keyID]; ok {
		return key, nil
	}
	return nil, authn.ErrInvalidSigningKey
}

func parseSigningKeys(raw string) (staticKeys, error) {
	var set jose.JSONWebKeySet
	if err := json.Unmarshal([]byte(raw), &set); err != nil {
		return nil, fmt.Errorf("decode JWKS: %w", err)
	}
	if len(set.Keys) == 0 {
		return nil, errors.New("JWKS has no keys")
	}
	keys := make(staticKeys, len(set.Keys))
	for i := range set.Keys {
		key := &set.Keys[i]
		switch {
		case key.KeyID == "":
			return nil, fmt.Errorf("key %d has no key ID", i)
		case !key.IsPublic():
			// A private key in the environment would let anyone who reads it sign tokens.
			return nil, fmt.Errorf("key %q is not a public key", key.KeyID)
		case !key.Valid():
			return nil, fmt.Errorf("key %q is invalid", key.KeyID)
		case key.Use != "" && key.Use != "sig":
			return nil, fmt.Errorf("key %q is not a signing key", key.KeyID)
		}
		if _, ok := keys[key.KeyID]; ok {
			return nil, fmt.Errorf("duplicate key ID %q", key.KeyID)
		}
		keys[key.KeyID] = key
	}
	return keys, nil
}
