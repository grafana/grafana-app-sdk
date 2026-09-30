package testing

// kvSourceManifest exercises searchFields whose value comes from a kv
// subresource document (source.kv) instead of a path in the resource.
// KVSourced mixes a plain path field with kv-sourced fields, one of them using
// a nested dotted path and a nested key. PlainSearch declares only path fields,
// so its fields must never gain a source.
kvSourceManifest: {
	appName: "kv-source-app"
	versions: {
		"v1": kvSourceManifestV1
	}
}

kvSourceManifestV1: {
	codegen: ts: enabled: false
	kinds: [
		{
			kind: "KVSourced"
			schema: spec: title: string
			kv: {}
			searchFields: [
				{
					name: "title"
					path: "spec.title"
					type: "string"
					capabilities: ["filter", "sort", "retrieve"]
				},
				{
					name: "views_total"
					type: "int64"
					capabilities: ["sort", "retrieve"]
					description: "Populated from kv."
					source: kv: {owner: "usageinsights.grafana.app", key: "stats", path: "views_total"}
				},
				{
					name: "nested_views"
					type: "int64"
					capabilities: ["sort"]
					source: kv: {owner: "other-owner.example.app", key: "daily/rollup", path: "totals.views"}
				},
			]
		},
		{
			kind: "PlainSearch"
			schema: spec: title: string
			searchFields: [
				{
					name: "title"
					path: "spec.title"
					type: "string"
					capabilities: ["filter"]
				},
			]
		},
	]
}
