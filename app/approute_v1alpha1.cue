package app

appRoutev1alpha1: appRouteKind & {
	// AppRoute's spec is the flat union of the AppManifest and RuntimeConfig
	// specs. DRY: reference both specs directly, never redeclare fields.
	// This only unifies cleanly because the two field sets are disjoint
	// (v1alpha3 dropped `operator`, which used to collide with RuntimeConfig's).
	schema: spec: appManifestv1alpha3.schema.spec & runtimeConfigv1alpha1.schema.spec
}
