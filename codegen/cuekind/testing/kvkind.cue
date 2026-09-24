package testing

// kvManifest is a minimal manifest used to test the kv subresource codegen path.
// It contains two kinds: one with an empty kv block (bare opt-in) and one with
// explicit limits. A third kind has no kv block to verify absence is preserved.
kvManifest: {
	appName: "kv-app"
	versions: {
		"v1": kvManifestV1
	}
}

kvManifestV1: {
	codegen: ts: enabled: false
	kinds: [
		{
			kind:   "KVEmpty"
			schema: spec: field: string
			kv:     {}
		},
		{
			kind:   "KVLimited"
			schema: spec: field: string
			kv: {
				maxValueBytes:   1024
				maxKeysPerOwner: 50
			}
		},
		{
			kind:   "NoKV"
			schema: spec: field: string
		},
	]
}
