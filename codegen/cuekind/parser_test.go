package cuekind

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/codegen"
)

func TestParseManifestTestApp(t *testing.T) {
	parser, err := NewParser(testingCue(t), false)
	require.NoError(t, err)

	manifest, err := parser.ParseManifest("testManifest")
	require.NoError(t, err)

	props := manifest.Properties()
	assert.Equal(t, "test-app", props.AppName)
	assert.Equal(t, "v1", props.PreferredVersion)

	require.NotNil(t, props.OperatorURL)
	assert.Equal(t, "https://foo.bar:8443", *props.OperatorURL)

	require.Len(t, props.ExtraPermissions.AccessKinds, 1)
	assert.Equal(t, "foo.bar", props.ExtraPermissions.AccessKinds[0].Group)
	assert.Equal(t, "foos", props.ExtraPermissions.AccessKinds[0].Resource)
	assert.Equal(t, []string{"get", "list", "watch"}, props.ExtraPermissions.AccessKinds[0].Actions)

	require.Contains(t, props.Roles, "test-app:reader")
	role := props.Roles["test-app:reader"]
	assert.Equal(t, "Test App Viewer", role.Title)
	assert.Equal(t, "View Test App Resources", role.Description)
	assert.Equal(t, []string{"createFoobar"}, role.Routes)

	require.NotNil(t, props.RoleBindings)
	assert.Equal(t, []string{"test-app:reader"}, props.RoleBindings.Viewer)

	versions := manifest.Versions()
	require.Len(t, versions, 3)
	assert.Equal(t, "v1", versions[0].Name())
	assert.Equal(t, "v2", versions[1].Name())
	assert.Equal(t, "v3", versions[2].Name())

	// v1 should have 2 kinds (testKind + testKind2)
	assert.Len(t, versions[0].Kinds(), 2)
	// v2 and v3 should each have 1 kind (testKind only)
	assert.Len(t, versions[1].Kinds(), 1)
	assert.Len(t, versions[2].Kinds(), 1)
}

func TestParseManifestKindProperties(t *testing.T) {
	parser, err := NewParser(testingCue(t), false)
	require.NoError(t, err)

	manifest, err := parser.ParseManifest("testManifest")
	require.NoError(t, err)

	versions := manifest.Versions()

	// v1 TestKind
	var testKind codegen.VersionedKind
	for _, k := range versions[0].Kinds() {
		if k.Kind == "TestKind" {
			testKind = k
			break
		}
	}
	assert.Equal(t, "TestKind", testKind.Kind)
	assert.Equal(t, "TestKinds", testKind.PluralName)
	assert.True(t, testKind.Conversion)
	assert.Equal(t, "http://foo.bar/convert", testKind.ConversionWebhookProps.URL)
	assert.Equal(t, []codegen.KindAdmissionCapabilityOperation{"create", "update"}, testKind.Validation.Operations)

	// v2 TestKind should have mutation and additional printer columns
	v2Kind := versions[1].Kinds()[0]
	assert.Equal(t, []codegen.KindAdmissionCapabilityOperation{"create", "update"}, v2Kind.Mutation.Operations)
	require.Len(t, v2Kind.AdditionalPrinterColumns, 1)
	assert.Equal(t, "STRING FIELD", v2Kind.AdditionalPrinterColumns[0].Name)
	assert.Equal(t, ".spec.stringField", v2Kind.AdditionalPrinterColumns[0].JSONPath)
}

func TestParseManifestRoutes(t *testing.T) {
	parser, err := NewParser(testingCue(t), false)
	require.NoError(t, err)

	t.Run("testManifest v3", func(t *testing.T) {
		manifest, err := parser.ParseManifest("testManifest")
		require.NoError(t, err)

		v3 := manifest.Versions()[2]

		// Version-level namespaced route
		routes := v3.Routes()
		require.Contains(t, routes.Namespaced, "/foobar")
		require.Contains(t, routes.Namespaced["/foobar"], "POST")
		assert.Equal(t, "createFoobar", routes.Namespaced["/foobar"]["POST"].Name)

		// Kind-level routes
		v3Kind := v3.Kinds()[0]
		require.Contains(t, v3Kind.Routes, "/reconcile")
		assert.Equal(t, "createReconcileRequest", v3Kind.Routes["/reconcile"]["POST"].Name)
		require.Contains(t, v3Kind.Routes, "/search")
		assert.Equal(t, "getTestKindSearchResult", v3Kind.Routes["/search"]["GET"].Name)
	})

	t.Run("integrationManifest", func(t *testing.T) {
		manifest, err := parser.ParseManifest("integrationManifest")
		require.NoError(t, err)

		assert.Equal(t, "integration", manifest.Properties().AppName)

		v1 := manifest.Versions()[0]
		routes := v1.Routes()

		// Namespaced route
		require.Contains(t, routes.Namespaced, "/foo")
		assert.Equal(t, "getFoo", routes.Namespaced["/foo"]["GET"].Name)

		// Cluster route
		require.Contains(t, routes.Cluster, "/bar")
		assert.Equal(t, "createBar", routes.Cluster["/bar"]["POST"].Name)

		// Kind-level route
		v1Kind := v1.Kinds()[0]
		assert.Equal(t, "Foo", v1Kind.Kind)
		require.Contains(t, v1Kind.Routes, "/details")
		assert.Equal(t, "getDetails", v1Kind.Routes["/details"]["GET"].Name)
		assert.True(t, v1Kind.Routes["/details"]["GET"].ResponseMetadata.ObjectMeta)
	})
}

func TestParseManifestCustomApp(t *testing.T) {
	parser, err := NewParser(testingCue(t), false)
	require.NoError(t, err)

	manifest, err := parser.ParseManifest("customManifest")
	require.NoError(t, err)

	props := manifest.Properties()
	assert.Equal(t, "custom-app", props.AppName)
	assert.Equal(t, "v1-0", props.PreferredVersion)

	versions := manifest.Versions()
	require.Len(t, versions, 2)
	for _, v := range versions {
		require.Len(t, v.Kinds(), 1)
		assert.Equal(t, "CustomKind", v.Kinds()[0].Kind)
	}
}

func TestParseManifestInvalidCases(t *testing.T) {
	parser, err := NewParser(testingCue(t), false)
	require.NoError(t, err)

	tests := []struct {
		name        string
		selector    string
		errContains string
	}{
		// Manifest-level validation
		{
			name:        "uppercase appName",
			selector:    "invalidAppNameUppercase",
			errContains: `appName: invalid value "BadApp"`,
		},
		{
			name:        "missing appName",
			selector:    "invalidAppNameMissing",
			errContains: `appName: cannot convert non-concrete value`,
		},
		{
			name:        "invalid groupOverride",
			selector:    "invalidGroupOverride",
			errContains: `groupOverride: invalid value "INVALID"`,
		},
		{
			name:        "empty versions",
			selector:    "invalidEmptyVersions",
			errContains: `preferredVersion: cannot convert non-concrete value`,
		},
		{
			name:        "nonexistent selector",
			selector:    "nonExistentManifest",
			errContains: "field not found: nonExistentManifest",
		},
		// Kind-level validation
		{
			name:        "lowercase kind name",
			selector:    "invalidKindNameLowercase",
			errContains: `kinds.0.kind: invalid value "lowercasekind"`,
		},
		{
			name:        "invalid scope",
			selector:    "invalidScope",
			errContains: `kinds.0.scope`,
		},
		{
			name:        "invalid pluralName",
			selector:    "invalidPluralName",
			errContains: `pluralName: 2 errors in empty disjunction`,
		},
		// Kind-level route validation
		{
			name:        "kind route missing name",
			selector:    "invalidRouteNameMissing",
			errContains: `routes."/noname".GET.name: cannot convert non-concrete value`,
		},
		{
			name:        "kind route name bad prefix",
			selector:    "invalidRouteNameBadPrefix",
			errContains: `routes."/broken".GET.name: invalid value "badName"`,
		},
		{
			name:        "kind route invalid method",
			selector:    "invalidRouteMethod",
			errContains: `routes."/test".INVALID: field not allowed`,
		},
		{
			name:        "kind route invalid extension key",
			selector:    "invalidExtensionKey",
			errContains: `extensions."not-x-prefixed": field not allowed`,
		},
		{
			name:        "printer column missing fields",
			selector:    "invalidPrinterColumnMissingFields",
			errContains: `additionalPrinterColumns.0.type: cannot convert non-concrete value`,
		},
		// Role validation (CUE pattern constraints don't enforce #Role, validated in Go)
		{
			name:        "role empty title",
			selector:    "invalidRoleEmptyTitle",
			errContains: `roles."role-app:reader".title: invalid value ""`,
		},
		// Version-level route validation
		{
			name:        "version namespaced route bad name",
			selector:    "invalidVersionRouteName",
			errContains: `routes.namespaced."/bad".POST.name: invalid value "notAValidPrefix"`,
		},
		{
			name:        "version namespaced route missing name",
			selector:    "invalidNamespacedRouteMissingName",
			errContains: `routes.namespaced."/noname".GET.name: cannot convert non-concrete value`,
		},
		{
			name:        "version cluster route bad name",
			selector:    "invalidClusterRouteBadName",
			errContains: `routes.cluster."/bad".POST.name: invalid value "invalidPrefix"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parser.ParseManifest(tt.selector)
			require.ErrorContains(t, err, tt.errContains)
		})
	}
}
