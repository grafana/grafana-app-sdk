package routes

import (
	"bytes"
	"encoding/json"
	"testing"
	"text/template"

	"github.com/grafana/authlib/types"
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
		{name: "incorrect field case", value: `{"ReqRole":"Admin"}`, wantErr: `unknown field "ReqRole"`},
		{name: "incorrect headers case", value: `{"Headers":[{"name":"X-Test"}]}`, wantErr: `unknown field "Headers"`},
		{name: "incorrect nested case", value: `{"tokenAuth":{"URL":"https://example.com"}}`, wantErr: "tokenAuth.URL"},
		{name: "path", value: `{"path":"/x"}`, wantErr: `unknown field "path"`},
		{name: "method", value: `{"method":"GET"}`, wantErr: `unknown field "method"`},
		{name: "URL type", value: `{"url":42}`, wantErr: "cannot unmarshal"},
		{name: "role", value: `{"reqRole":"SuperAdmin"}`, wantErr: "invalid role value"},
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
		{name: "header typo", value: `{"headers":[{"name":"X-Test","contents":"x"}]}`, wantErr: `unknown field "headers[0].contents"`},
		{name: "missing param name", value: `{"urlParams":[{"content":"x"}]}`, wantErr: "urlParams[0].name"},
		{name: "missing param content", value: `{"urlParams":[{"name":"x"}]}`, wantErr: "urlParams[0].content is required"},
		{name: "token type", value: `{"tokenAuth":true}`, wantErr: "cannot unmarshal"},
		{name: "scope type", value: `{"jwtTokenAuth":{"scopes":[1]}}`, wantErr: "cannot unmarshal"},
		{name: "param type", value: `{"tokenAuth":{"params":{"key":1}}}`, wantErr: "cannot unmarshal"},
		{name: "token typo", value: `{"tokenAuth":{"uri":"https://example.com"}}`, wantErr: `unknown field "tokenAuth.uri"`},
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
	want := ProxyTarget{ReqRole: types.RoleEditor, URL: "{{.URL}}", Body: json.RawMessage(`{ "key" : "{{.JsonData.key}}" }`)}
	op := &spec3.Operation{}
	op.AddExtension(ExtensionProxy, want)
	target, declared, err := ProxyTargetFromOperation(op)
	require.NoError(t, err)
	require.True(t, declared)
	require.Equal(t, types.RoleEditor, target.ReqRole)
	require.Equal(t, want.URL, target.URL)
	require.JSONEq(t, string(want.Body), string(target.Body))
}

func TestProxyTargetRoleType(t *testing.T) {
	for _, tt := range []struct {
		name, input string
		want        types.RoleType
		wantErr     bool
	}{
		{name: "omitted", input: `{}`},
		{name: "empty defaults to viewer", input: `{"reqRole":""}`, want: types.RoleViewer},
		{name: "none", input: `{"reqRole":"None"}`, want: types.RoleNone},
		{name: "viewer", input: `{"reqRole":"Viewer"}`, want: types.RoleViewer},
		{name: "editor", input: `{"reqRole":"Editor"}`, want: types.RoleEditor},
		{name: "admin", input: `{"reqRole":"Admin"}`, want: types.RoleAdmin},
		{name: "lowercase none", input: `{"reqRole":"none"}`, want: types.RoleNone},
		{name: "lowercase viewer", input: `{"reqRole":"viewer"}`, want: types.RoleViewer},
		{name: "lowercase editor", input: `{"reqRole":"editor"}`, want: types.RoleEditor},
		{name: "lowercase admin", input: `{"reqRole":"admin"}`, want: types.RoleAdmin},
		{name: "uppercase", input: `{"reqRole":"ADMIN"}`, wantErr: true},
		{name: "whitespace", input: `{"reqRole":" Admin "}`, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			op := &spec3.Operation{}
			op.AddExtension(ExtensionProxy, json.RawMessage(tt.input))
			target, declared, err := ProxyTargetFromOperation(op)
			require.True(t, declared)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, target.ReqRole)
		})
	}
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

func TestProxyTargetPreservesBodyTemplate(t *testing.T) {
	body := json.RawMessage("{ \"match\": \"{{if eq .value `<`}}yes{{else}}no{{end}}\" }")
	want := ProxyTarget{Body: body}
	for _, value := range []any{want, &want, json.RawMessage(`{"body":` + string(body) + `}`)} {
		op := &spec3.Operation{}
		op.AddExtension(ExtensionProxy, value)
		target, declared, err := ProxyTargetFromOperation(op)
		require.NoError(t, err)
		require.True(t, declared)
		require.Equal(t, body, target.Body)
		tmpl, err := template.New("body").Parse(string(target.Body))
		require.NoError(t, err)
		var rendered bytes.Buffer
		require.NoError(t, tmpl.Execute(&rendered, map[string]string{"value": "<"}))
		require.JSONEq(t, `{"match":"yes"}`, rendered.String())
	}
	var decoded any
	require.NoError(t, json.Unmarshal([]byte(`{"body":`+string(body)+`}`), &decoded))
	op := &spec3.Operation{}
	op.AddExtension(ExtensionProxy, decoded)
	target, _, err := ProxyTargetFromOperation(op)
	require.NoError(t, err)
	require.Contains(t, string(target.Body), "`<`", "decoded extensions must not HTML-escape template expressions")
}

func TestProxyTargetTypedValidation(t *testing.T) {
	for _, value := range []any{
		(*ProxyTarget)(nil),
		ProxyTarget{Body: json.RawMessage(`{"invalid":`)},
		&ProxyTarget{ReqRole: "SuperAdmin"},
		ProxyTarget{AuthType: ProxyAuthTypeAzure},
		ProxyTarget{URL: "{{"},
	} {
		op := &spec3.Operation{}
		op.AddExtension(ExtensionProxy, value)
		target, declared, err := ProxyTargetFromOperation(op)
		require.ErrorContains(t, err, "invalid "+ExtensionProxy)
		require.True(t, declared)
		require.Nil(t, target)
	}
}

func TestProxyTargetRawJSON(t *testing.T) {
	for _, tt := range []struct {
		name, value, wantErr string
	}{
		{name: "large body number", value: `{"body":1e1000}`},
		{name: "duplicate field", value: `{"reqRole":"Admin","reqRole":"Viewer"}`, wantErr: "duplicate field"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			op := &spec3.Operation{}
			op.AddExtension(ExtensionProxy, json.RawMessage(tt.value))
			target, declared, err := ProxyTargetFromOperation(op)
			require.True(t, declared)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Nil(t, target)
				return
			}
			require.NoError(t, err)
			require.Equal(t, json.RawMessage(`1e1000`), target.Body)
		})
	}
}

func TestProxyTargetNormalizesRoleAndEscapes(t *testing.T) {
	op := &spec3.Operation{OperationProps: spec3.OperationProps{}}
	op.Extensions = spec.Extensions{}
	op.Extensions[ExtensionProxy] = ProxyTarget{ReqRole: "editor"}
	target, _, err := ProxyTargetFromOperation(op)
	require.NoError(t, err)
	require.Equal(t, types.RoleEditor, target.ReqRole)

	// json.Marshal-style HTML escapes are undone, but an escaped backslash is not.
	op.Extensions[ExtensionProxy] = json.RawMessage(`{"body":{"a":"{{if eq .X ` + "`\\u003c`" + `}}&{{end}}","b":"\\u003c","n":9007199254740993}}`)
	target, _, err = ProxyTargetFromOperation(op)
	require.NoError(t, err)
	require.JSONEq(t, `{"a":"{{if eq .X `+"`<`"+`}}&{{end}}","b":"\\u003c","n":9007199254740993}`, string(target.Body))
	require.Contains(t, string(target.Body), "`<`")
}
