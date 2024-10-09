package app

manifest: {
	kind: "AppManifest"
	group: "apps"
	apiResource: {
		groupOverride: "apps.grafana.com"
		scope: "Cluster"
	}
	codegen: {
		frontend: false
		backend: true
	}
	current: "v1alpha1"
	versions: {}
}

manifest: versions: v1alpha1: {
	schema: {
		#AdditionalPrinterColumns: {
			// name is a human readable name for the column.
			name: string
			// type is an OpenAPI type definition for this column.
			// See https://github.com/OAI/OpenAPI-Specification/blob/master/versions/2.0.md#data-types for details.
			type: string
			// format is an optional OpenAPI type definition for this column. The 'name' format is applied
			// to the primary identifier column to assist in clients identifying column is the resource name.
			// See https://github.com/OAI/OpenAPI-Specification/blob/master/versions/2.0.md#data-types for details.
			format?: string
			// description is a human readable description of this column.
			description?: string
			// priority is an integer defining the relative importance of this column compared to others. Lower
			// numbers are considered higher priority. Columns that may be omitted in limited space scenarios
			// should be given a priority greater than 0.
			priority?: int32
			// jsonPath is a simple JSON path (i.e. with array notation) which is evaluated against
			// each custom resource to produce the value for this column.
			jsonPath: string
		}
		#AdmissionOperation: "CREATE" | "UPDATE" | "DELETE" | "CONNECT" | "*"
		#ValidationCapability: {
			operations: [...#AdmissionOperation]
		}
		#MutationCapability: {
			operations: [...#AdmissionOperation]
		}
		#AdmissionCapabilities: {
			validation?: #ValidationCapability
			mutation?: #MutationCapability
		}
		#ManifestKindVersion: {
			name: string
			admission?: #AdmissionCapabilities
			schema: {
				[string]: _
			}
			selectableFields: [...string]
			additionalPrinterColumns?: [...#AdditionalPrinterColumns]
		}
		#ManifestKind: {
			kind: string
			scope: string
			conversion: bool
			versions: [...#ManifestKindVersion]
		}
		spec: {
			appName: string
			group: string
			kinds: [...#ManifestKind]
		}
	}
}