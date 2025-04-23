package app

manifest: {
	appName: "app-manifest"
	groupOverride: "apps.grafana.com"
	kinds: [appManifest]
	extraPermissions: {
		accessKinds: [{
			group: "apiextensions.k8s.io",
			resource: "customresourcedefinitions",
			actions: ["get","list","create","update","delete","watch"],
		}]
	}
}

appManifest: {
	kind: "AppManifest"
	scope: "Cluster"
	codegen: {
		ts: enabled: false
	}
	current: "v1alpha1"
	versions: {}
}

appManifest: versions: v1alpha1: {
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
		#AdmissionOperation: "CREATE" | "UPDATE" | "DELETE" | "CONNECT" | "*" @cog(kind="enum",memberNames="create|update|delete|connect|all")
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
		#ManifestKindVersionSchema: {
			[string]: _
		}
		#ManifestKindVersion: {
			name: string
			admission?: #AdmissionCapabilities
			schema: #ManifestKindVersionSchema
			selectableFields?: [...string]
			additionalPrinterColumns?: [...#AdditionalPrinterColumns]
		}
		#ManifestKind: {
			kind: string
			scope: string
			conversion: bool
			versions: [...#ManifestKindVersion]
		}
		#KindPermission: {
			group: string
			resource: string
			actions: [...string]
		}
		#OperatorWebhookProperties: {
			conversionPath?: string | *"/convert"
			validationPath?: string | *"/validate"
			mutationPath?: string | *"/mutate"
		}
		#OperatorInfo: {
			// URL is the URL of the operator's HTTPS endpoint, including port if non-standard (443).
			// It should be a URL which the API server can access.
			url?: string
			// Webhooks contains information about the various webhook paths.
			webhooks?: #OperatorWebhookProperties
		}
		spec: {
			appName: string
			group: string
			kinds: [...#ManifestKind]
			// ExtraPermissions contains additional permissions needed for an app's backend component to operate.
			// Apps implicitly have all permissions for kinds they managed (defined in `kinds`).
			extraPermissions?: {
				// accessKinds is a list of KindPermission objects for accessing additional kinds provided by other apps
				accessKinds: [...#KindPermission]
			}
			// DryRunKinds dictates whether this revision should create/update CRD's from the provided kinds,
			// Or simply validate and report errors in status.resources.crds.
			// If dryRunKinds is true, CRD change validation will be skipped on ingress and reported in status instead.
			// Even if no validation errors exist, CRDs will not be created or updated for a revision with dryRunKinds=true.
			dryRunKinds?: bool | *false
			// Operator has information about the operator being run for the app, if there is one.
			// When present, it can indicate to the API server the URL and paths for webhooks, if applicable.
			// This is only required if you run your app as an operator and any of your kinds support webhooks for validation,
			// mutation, or conversion.
			operator?: #OperatorInfo
		}
		status: {
			#ApplyStatus: {
				status: "success" | "failure"
				// details may contain specific information (such as error message(s)) on the reason for the status
				details?: string
			}
			// ObservedGeneration is the last generation which has been applied by the controller.
			observedGeneration?: int
			// Resources contains the status of each resource type created or updated in the API server
			// as a result of the AppManifest.
			resources: {
				[string]: #ApplyStatus
			}
		}
	}
}