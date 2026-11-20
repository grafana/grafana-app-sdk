package routes

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

func TestProxyTargetFromOperation(t *testing.T) {
	for _, tt := range []struct {
		name, value, wantErr string
	}{
		{name: "data source URL", value: `{}`},
		{name: "templates", value: `{
			"url":"{{.URL}}/api/{{.JsonData.project}}", "reqRole":"Editor", "reqAction":"datasources:query",
			"headers":[{"name":"Authorization","content":"Bearer {{.SecureJsonData.token | orEmpty}}"}],
			"urlParams":[{"name":"{{.JsonData.param}}","content":"{{.JsonData.value}}"}],
			"body":{"id":"{{.JsonData.id}}"}
		}`},
		{name: "token", value: `{"tokenAuth":{"url":"https://example.com/token","scopes":["read"],"params":{"client_secret":"{{.SecureJsonData.secret}}"}}}`},
		{name: "azure", value: `{"authType":"azure","tokenAuth":{"params":{"resource":"{{.JsonData.resource}}"}}}`},
		{name: "gce", value: `{"authType":"gce","jwtTokenAuth":{}}`},
		{name: "jwt", value: `{"authType":"jwt","jwtTokenAuth":{"params":{"private_key":"{{.SecureJsonData.key}}"}}}`},
		{name: "both token configs", value: `{"tokenAuth":{},"jwtTokenAuth":{}}`},
		{name: "null body", value: `{"body":null}`},
		{name: "array body", value: `{"body":["{{.JsonData.id}}"]}`},
		{name: "null", value: `null`, wantErr: "must be an object"},
		{name: "scalar", value: `true`, wantErr: "cannot unmarshal"},
		{name: "array", value: `[]`, wantErr: "cannot unmarshal"},
		{name: "unknown option", value: `{"reqRol":"Admin"}`, wantErr: `unknown field "reqRol"`},
		{name: "path", value: `{"path":"/x"}`, wantErr: `unknown field "path"`},
		{name: "method", value: `{"method":"GET"}`, wantErr: `unknown field "method"`},
		{name: "URL type", value: `{"url":42}`, wantErr: "cannot unmarshal"},
		{name: "role", value: `{"reqRole":"admin"}`, wantErr: "reqRole must be"},
		{name: "null role", value: `{"reqRole":null}`, wantErr: "reqRole must not be null"},
		{name: "action type", value: `{"reqAction":true}`, wantErr: "cannot unmarshal"},
		{name: "auth type", value: `{"authType":"oauth"}`, wantErr: "authType must be"},
		{name: "azure missing token", value: `{"authType":"azure"}`, wantErr: "requires tokenAuth"},
		{name: "gce missing token", value: `{"authType":"gce"}`, wantErr: "requires jwtTokenAuth"},
		{name: "jwt missing token", value: `{"authType":"jwt"}`, wantErr: "requires jwtTokenAuth"},
		{name: "headers type", value: `{"headers":{}}`, wantErr: "cannot unmarshal"},
		{name: "header name", value: `{"headers":[{"name":"bad name","content":"x"}]}`, wantErr: "headers[0].name"},
		{name: "missing header name", value: `{"headers":[{"content":"x"}]}`, wantErr: "headers[0].name"},
		{name: "missing header content", value: `{"headers":[{"name":"X-Test"}]}`, wantErr: "headers[0].content is required"},
		{name: "null header", value: `{"headers":[null]}`, wantErr: "headers[0] must not be null"},
		{name: "header typo", value: `{"headers":[{"name":"X-Test","contents":"x"}]}`, wantErr: `unknown field "contents"`},
		{name: "missing param name", value: `{"urlParams":[{"content":"x"}]}`, wantErr: "urlParams[0].name"},
		{name: "missing param content", value: `{"urlParams":[{"name":"x"}]}`, wantErr: "urlParams[0].content is required"},
		{name: "token type", value: `{"tokenAuth":true}`, wantErr: "cannot unmarshal"},
		{name: "scope type", value: `{"jwtTokenAuth":{"scopes":[1]}}`, wantErr: "cannot unmarshal"},
		{name: "param type", value: `{"tokenAuth":{"params":{"key":1}}}`, wantErr: "cannot unmarshal"},
		{name: "token typo", value: `{"tokenAuth":{"uri":"https://example.com"}}`, wantErr: `unknown field "uri"`},
		{name: "URL template", value: `{"url":"{{"}`, wantErr: "url:"},
		{name: "header template", value: `{"headers":[{"name":"X-Test","content":"{{unknown}}"}]}`, wantErr: "headers[0].content:"},
		{name: "param name template", value: `{"urlParams":[{"name":"{{","content":"x"}]}`, wantErr: "urlParams[0].name:"},
		{name: "param content template", value: `{"urlParams":[{"name":"x","content":"{{"}]}`, wantErr: "urlParams[0].content:"},
		{name: "token URL template", value: `{"tokenAuth":{"url":"{{"}}`, wantErr: "tokenAuth.url:"},
		{name: "token param template", value: `{"jwtTokenAuth":{"params":{"key":"{{"}}}`, wantErr: "jwtTokenAuth.params.key:"},
		{name: "body template", value: `{"body":{"key":"{{"}}`, wantErr: "body:"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var raw any
			require.NoError(t, json.Unmarshal([]byte(tt.value), &raw))
			op := &spec3.Operation{VendorExtensible: spec.VendorExtensible{Extensions: spec.Extensions{ExtensionProxy: raw}}}
			target, declared, err := ProxyTargetFromOperation(op)
			require.True(t, declared)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, "invalid "+ExtensionProxy)
				require.ErrorContains(t, err, tt.wantErr)
				require.Nil(t, target)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, target)
			encoded, err := json.Marshal(target)
			require.NoError(t, err)
			require.JSONEq(t, tt.value, string(encoded))
		})
	}
}

func TestProxyTargetTypedAndAbsent(t *testing.T) {
	for _, op := range []*spec3.Operation{nil, {}} {
		target, declared, err := ProxyTargetFromOperation(op)
		require.NoError(t, err)
		require.False(t, declared)
		require.Nil(t, target)
	}
	want := ProxyTarget{URL: "{{.URL}}", Body: json.RawMessage(`{ "key" : "{{.JsonData.key}}" }`)}
	op := &spec3.Operation{}
	op.AddExtension(ExtensionProxy, want)
	target, declared, err := ProxyTargetFromOperation(op)
	require.NoError(t, err)
	require.True(t, declared)
	require.Equal(t, want.URL, target.URL)
	require.JSONEq(t, string(want.Body), string(target.Body))
}

func TestParseRejectsInvalidProxyOperations(t *testing.T) {
	invalid := &spec3.Operation{}
	invalid.AddExtension(ExtensionProxy, map[string]any{"reqRole": "SuperAdmin"})
	version := testVersion(map[string]spec3.PathProps{
		"/proxy": {Get: &spec3.Operation{}, Head: invalid, Post: &spec3.Operation{}},
	})
	routes, problems := Parse(version, Options{})
	require.Len(t, routes, 1)
	require.Nil(t, routes[0].Operations.Get)
	require.Nil(t, routes[0].Operations.Head)
	require.NotNil(t, routes[0].Operations.Post)
	require.Contains(t, reasons(problems)["HEAD /proxy"], ExtensionProxy)
	require.Contains(t, reasons(problems)["GET /proxy"], "would also answer HEAD")
	require.NotNil(t, version.OpenAPI.Paths["/proxy"].Get, "parsing must not change the manifest")
}
