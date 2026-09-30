package testing

// Invalid source.kv declarations. Each selector is a whole manifest so the
// parser and the manifest generator can be exercised independently.

// path and source are mutually exclusive.
invalidKVSourcePathAndSource: {
	appName: "kv-source-both"
	versions: "v1": {
		codegen: ts: enabled: false
		kinds: [{
			kind: "Both"
			schema: spec: title: string
			kv: {}
			searchFields: [{
				name: "views_total"
				path: "spec.title"
				type: "string"
				capabilities: ["sort"]
				source: kv: {owner: "usageinsights.grafana.app", key: "stats", path: "views_total"}
			}]
		}]
	}
}

// kv-sourced fields must not be arrays.
invalidKVSourceArray: {
	appName: "kv-source-array"
	versions: "v1": {
		codegen: ts: enabled: false
		kinds: [{
			kind: "ArrayKV"
			schema: spec: title: string
			kv: {}
			searchFields: [{
				name: "views_total"
				type: "int64"
				array: true
				capabilities: ["sort"]
				source: kv: {owner: "usageinsights.grafana.app", key: "stats", path: "views_total"}
			}]
		}]
	}
}

// source.kv.path must be a dotted path: no [*] projection.
invalidKVSourcePathProjection: {
	appName: "kv-source-projection"
	versions: "v1": {
		codegen: ts: enabled: false
		kinds: [{
			kind: "ProjectionKV"
			schema: spec: title: string
			kv: {}
			searchFields: [{
				name: "views_total"
				type: "int64"
				capabilities: ["sort"]
				source: kv: {owner: "usageinsights.grafana.app", key: "stats", path: "a[*].b"}
			}]
		}]
	}
}

// source.kv.path must not start with a dot.
invalidKVSourcePathLeadingDot: {
	appName: "kv-source-leading-dot"
	versions: "v1": {
		codegen: ts: enabled: false
		kinds: [{
			kind: "LeadingDotKV"
			schema: spec: title: string
			kv: {}
			searchFields: [{
				name: "views_total"
				type: "int64"
				capabilities: ["sort"]
				source: kv: {owner: "usageinsights.grafana.app", key: "stats", path: ".a"}
			}]
		}]
	}
}

// source.kv.owner must match ^[a-z0-9.-]+$.
invalidKVSourceOwner: {
	appName: "kv-source-owner"
	versions: "v1": {
		codegen: ts: enabled: false
		kinds: [{
			kind: "OwnerKV"
			schema: spec: title: string
			kv: {}
			searchFields: [{
				name: "views_total"
				type: "int64"
				capabilities: ["sort"]
				source: kv: {owner: "Usage Insights", key: "stats", path: "views_total"}
			}]
		}]
	}
}

// source.kv.key must not have an empty path segment.
invalidKVSourceKey: {
	appName: "kv-source-key"
	versions: "v1": {
		codegen: ts: enabled: false
		kinds: [{
			kind: "KeyKV"
			schema: spec: title: string
			kv: {}
			searchFields: [{
				name: "views_total"
				type: "int64"
				capabilities: ["sort"]
				source: kv: {owner: "usageinsights.grafana.app", key: "stats//daily", path: "views_total"}
			}]
		}]
	}
}

// source must name a kv source: an empty source is not a declaration.
invalidKVSourceEmpty: {
	appName: "kv-source-empty"
	versions: "v1": {
		codegen: ts: enabled: false
		kinds: [{
			kind: "EmptyKV"
			schema: spec: title: string
			kv: {}
			searchFields: [{
				name: "views_total"
				type: "int64"
				capabilities: ["sort"]
				source: {}
			}]
		}]
	}
}

// source.kv.key must be lower case.
invalidKVSourceKeyCase: {
	appName: "kv-source-key-case"
	versions: "v1": {
		codegen: ts: enabled: false
		kinds: [{
			kind: "KeyCaseKV"
			schema: spec: title: string
			kv: {}
			searchFields: [{
				name: "views_total"
				type: "int64"
				capabilities: ["sort"]
				source: kv: {owner: "usageinsights.grafana.app", key: "Stats", path: "views_total"}
			}]
		}]
	}
}
