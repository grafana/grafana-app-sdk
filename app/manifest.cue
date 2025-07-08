package app

manifest: {
	appName: "app-manifest"
	groupOverride: "apps.grafana.com"
	versions: {
		"v1alpha1": {
			codegen: ts: enabled: false
			kinds: [appManifestv1alpha1]
		}
	}
	extraPermissions: {
		accessKinds: [{
			group: "apiextensions.k8s.io",
			resource: "customresourcedefinitions",
			actions: ["get","list","create","update","delete","watch"],
		}]
	}
}

appManifestKind: {
	kind: "AppManifest"
	scope: "Cluster"
	codegen: {
		ts: enabled: false
	}
}

appManifestv1alpha1: appManifestKind & {
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
		#ManifestVersionKindSchema: {
			[string]: _
		}
		#ManifestVersionKind: {
			// Kind is the name of the kind. This should begin with a capital letter and be CamelCased
			kind: string
			// Plural is the plural version of `kind`. This is optional and defaults to the kind + "s" if not present.
			plural?: string
			// Scope dictates the scope of the kind. This field must be the same for all versions of the kind.
			// Different values will result in an error or undefined behavior.
			scope: *"Namespaced" | "Cluster"
			admission?: #AdmissionCapabilities
			schema: #ManifestVersionKindSchema
			selectableFields?: [...string]
			additionalPrinterColumns?: [...#AdditionalPrinterColumns]
			// Conversion indicates whether this kind supports custom conversion behavior exposed by the Convert method in the App.
			// It may not prevent automatic conversion behavior between versions of the kind when set to false
			// (for example, CRDs will always support simple conversion, and this flag enables webhook conversion).
			// This field should be the same for all versions of the kind. Different values will result in an error or undefined behavior.
			conversion?: bool | *false
		}
		#ManifestVersion: {
			// Name is the version name string, such as "v1" or "v1alpha1"
			name: string
			// Served dictates whether this version is served by the API server.
			// A version cannot be removed from a manifest until it is no longer served.
			served?: bool | *true
			// Kinds is a list of all the kinds served in this version.
			// Generally, kinds should exist in each version unless they have been deprecated (and no longer exist in a newer version)
			// or newly added (and didn't exist for older versions).
			kinds: [...#ManifestVersionKind]
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
			// Versions is the list of versions for this manifest, in order.
			versions: [...#ManifestVersion]
			// PreferredVersion is the preferred version for API use. If empty, it will use the latest from versions.
			// For CRDs, this also dictates which version is used for storage.
			preferredVersion?: string
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
