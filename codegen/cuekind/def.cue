package cuekind

import (
	"list"
	"strings"
	"struct"
	"time"
)

// _kubeObjectMetadata is metadata found in a kubernetes object's metadata field.
// It is not exhaustive and only includes fields which may be relevant to a kind's implementation,
// As it is also intended to be generic enough to function with any API Server.
_kubeObjectMetadata: {
	uid:                string
	creationTimestamp:  string & time.Time
	deletionTimestamp?: string & time.Time
	finalizers: [...string]
	resourceVersion: string
	generation:      int64
	labels: {
		[string]: string
	}
}

Schema: {
	// metadata contains embedded CommonMetadata and can be extended with custom string fields
	// TODO: use CommonMetadata instead of redefining here; currently needs to be defined here
	// without external reference as using the CommonMetadata reference breaks thema codegen.
	metadata: {
		_kubeObjectMetadata

		updateTimestamp: string & time.Time
		createdBy:       string
		updatedBy:       string
	} & {
		// All extensions to this metadata need to have string values (for APIServer encoding-to-annotations purposes)
		// Can't use this as it's not yet enforced CUE:
		//...string
		// Have to do this gnarly regex instead
		[!~"^(uid|creationTimestamp|deletionTimestamp|finalizers|resourceVersion|generation|labels|updateTimestamp|createdBy|updatedBy|extraFields)$"]: string
	}

	spec:   _

	// cuetsy is not happy creating spec with the MinFields constraint directly
	_specIsNonEmpty: spec & struct.MinFields(0)
}

SchemaWithOperatorState: Schema & {
	status: _ & {
		#OperatorState: {
			// lastEvaluation is the ResourceVersion last evaluated
			lastEvaluation: string
			// state describes the state of the lastEvaluation.
			// It is limited to three possible states for machine evaluation.
			state: "success" | "in_progress" | "failed"
			// descriptiveState is an optional more descriptive state field which has no requirements on format
			descriptiveState?: string
			// details contains any extra information that is operator-specific
			details?: {
				[string]: _
			}
		}

		// operatorStates is a map of operator ID to operator state evaluations.
		// Any operator which consumes this kind SHOULD add its state evaluation information to this field.
		operatorStates?: {
			[string]: #OperatorState
		}
		// additionalFields is reserved for future use
		additionalFields?: {
			[string]: _
		}
	}
}

#AdmissionCapability: {
	operations: [...string]
}
#CustomRouteRequest: {
    query?: _
    body?: _
}
#CustomRouteResponse: _
#CustomRouteResponseMetadata: {
		typeMeta: bool | *true
		listMeta: bool | *false
		objectMeta: bool | *false
}
#CustomRoute: {
		name?: =~ "^(get|log|read|replace|patch|delete|deletecollection|watch|connect|proxy|list|create|patch)([A-Za-z0-9]+)$"
    request: #CustomRouteRequest
    response: #CustomRouteResponse
    // responseMetadata allows codegen to include kubernetes metadata in the generated response object.
    // It is also copied into the AppManifest responseMetadata for use in kube-OpenAPI generation.
    responseMetadata: #CustomRouteResponseMetadata
    // extensions are all openAPI extensions that you wish to apply to this route.
    extensions: {
    	[=~"^x-(.+)$"]: _
    }
}
#CustomRoutePath: string
#CustomRouteMethod: "GET" | "POST" | "PUT" | "DELETE" | "PATCH" | "*"
#CustomRouteCapability: {
	[#CustomRoutePath]: {
		[#CustomRouteMethod]: #CustomRoute
	}
}
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

// Kind represents an arbitrary kind which can be used for code generation
Kind: S={
	kind:  =~"^([A-Z][a-zA-Z0-9-]{0,61}[a-zA-Z0-9])$"
	group: =~"^([a-z][a-z0-9-.]{0,61}[a-z0-9])$"
	// manifestGroup is a group shortname used for package naming in codegen
	// TODO: remove this when all jenny pipelines use the manifest, or keep around for convenience?
	manifestGroup: string
	// scope determines whether resources of this kind exist globally ("Cluster") or
	// within Kubernetes namespaces.
	scope: "Cluster" | *"Namespaced"
	// validation determines whether there is code-based validation for this kind.
	validation: #AdmissionCapability | *{
		operations: []
	}
	// mutation determines whether there is code-based mutation for this kind.
	mutation: #AdmissionCapability | *{
		operations: []
	}
	// conversion determines whether there is code-based conversion for this kind.
	conversion: bool | *false
	// conversionWebhookProps is a temporary way of specifying the service webhook information
	// which will be migrated away from once manifests are used in the codegen pipeline
	conversionWebhookProps: {
		url: string | *""
	}
	machineName:       strings.ToLower(strings.Replace(S.kind, "-", "_", -1))
	pluralName:        =~"^([A-Z][a-zA-Z0-9-]{0,61}[a-zA-Z])$" | *(S.kind + "s")
	pluralMachineName: strings.ToLower(strings.Replace(S.pluralName, "-", "_", -1))
	// codegen contains properties specific to generating code using tooling. At the root level of the kind, it sets
	// the defaults for the `codegen` field in all entries in `versions`. 
	// Valus set in `versions[x]: codegen` will overwrite the value set here.
	codegen: {
		// ts is the section for TypeScript codegen
		ts: {
			// enabled indicates whether front-end TypeScript code should be generated for this kind's schema
			enabled: bool | *S._codegen.ts.enabled
			// config is code generation configuration specific to TypeScript.
			// Currently, these config options are passed directly to grafana/cog when generating TypeScript
			config: {
				// importsMap associates package names to their import path.
				importsMap: {
					[string]: string
				}
				// enumsAsUnionTypes generates enums as a union of values instead of using
				// an actual `enum` declaration.
				// If EnumsAsUnionTypes is false, an enum will be generated as:
				// "`ts
				// enum Direction {
				//   Up = "up",
				//   Down = "down",
				//   Left = "left",
				//   Right = "right",
				// }
				// "`
				// If EnumsAsUnionTypes is true, the same enum will be generated as:
				// "`ts
				// type Direction = "up" | "down" | "left" | "right";
				// "`
				enumsAsUnionTypes: bool | *false
			} | *S._codegen.ts.config
		}
		// go is the section for go codegen
		go: {
			// enabled indicates whether back-end Go code should be generated for this kind's schema
			enabled: bool | *S._codegen.go.enabled
			config: {
			} | *S._codegen.go.config
		}
	}

	schema: _
	selectableFields: [...string]
	// additionalPrinterColumns is a list of additional columns to be printed in kubectl output
	additionalPrinterColumns?: [...#AdditionalPrinterColumns]
	// routes is a map of path patterns to custom routes that will be exposed as subresources for this kind.
	// entries here should not conflict with subresources (like spec and status) in the schema for the kind.
	routes?: #CustomRouteCapability

	_computedGroupKind: S.machineName+"."+group & =~"^([a-z][a-z0-9-.]{0,63}[a-z0-9])$"
}

Version: S={
	name: string
	// served dictates whether this version is served by the apiserver
	served: bool | *true
	// codegen contains properties specific to generating code using tooling. At the root level of the version, it sets
	// the defaults for the `codegen` field in all entries in `kinds`.
	// Valus set in `kinds[x]: codegen` will overwrite the value set here.
	codegen: {
		// ts is the section for TypeScript codegen
		ts: {
			// enabled indicates whether front-end TypeScript code should be generated for this kind's schema
			enabled: bool | *true
			// config is code generation configuration specific to TypeScript.
			// Currently, these config options are passed directly to grafana/cog when generating TypeScript
			config: {
				// importsMap associates package names to their import path.
				importsMap: {
					[string]: string
				}
				// enumsAsUnionTypes generates enums as a union of values instead of using
				// an actual `enum` declaration.
				// If EnumsAsUnionTypes is false, an enum will be generated as:
				// "`ts
				// enum Direction {
				//   Up = "up",
				//   Down = "down",
				//   Left = "left",
				//   Right = "right",
				// }
				// "`
				// If EnumsAsUnionTypes is true, the same enum will be generated as:
				// "`ts
				// type Direction = "up" | "down" | "left" | "right";
				// "`
				enumsAsUnionTypes: bool | *false
			}
		}
		// go is the section for go codegen
		go: {
			// enabled indicates whether back-end Go code should be generated for this kind's schema
			enabled: bool | *true
			config: {}
		}
	}
	kinds: [...{
		group:         S.fullGroup
		manifestGroup: S.group
		_codegen: S.codegen
	} & Kind]
	// routes is a map of path patterns to custom routes that will be exposed as resources for this version.
	// entries here should not conflict with the plural names for any kinds for this version.
	routes?: {
		// namespaced is the map of namespaced custom routes (includes a namespace in the path)
		namespaced: #CustomRouteCapability
		// cluster is the map of cluster-scoped custom routes (no namespace in the path)
		cluster: #CustomRouteCapability
	}
}

#AccessKind: {
	group:    string
	resource: string
	actions: [...string]
}

Manifest: S={
	appName: =~"^([a-z][a-z0-9-]*[a-z0-9])$"
	group:   strings.ToLower(strings.Replace(S.appName, "-", "", -1))
	versions: {
		[V=string]: {
			name: V
			group:         S.fullGroup
			manifestGroup: S.group
		} & Version
	}
	_allVersions: [for key, _ in S.versions { key }]
	preferredVersion: string | *(list.Sort(S._allVersions, list.Descending)[0])
	extraPermissions: {
		accessKinds: [...#AccessKind]
	}

	// operatorURL is the HTTPS URL of your operator, including port if non-standard (443).
	// If you do not deploy an operator, or if your operator does not expose an HTTPS server for webhooks, this can be omitted.
	// This is used to construct validation, mutations, or conversion webhooks for your deployment.
	operatorURL?: string

	// groupOverride is used to override the auto-generated group of "<group>.ext.grafana.app"
	// if present, this value is used for the full group instead.
	// groupOverride must have at least two parts (i.e. 'foo.bar'), but can be longer.
	// The length of fullGroup + kind name (for each kind) cannot exceed 62 characters
	groupOverride?: =~"^([a-z][a-z0-9-.]{0,48}[a-z0-9])\\.([a-z][a-z0-9-]{0,48}[a-z0-9])$"

	// _computedGroups is a list of groups computed from information in the plugin trait.
	// The first element is always the "most correct" one to use.
	// This field could be inlined into `group`, but is separate for clarity.
	_computedGroups: [
		if S.groupOverride != _|_ {
			strings.ToLower(S.groupOverride)
		},
		strings.ToLower(strings.Replace(S.group, "_", "-", -1)) + ".ext.grafana.com", // TODO: change to ext.grafana.app?
	]

	// fullGroup is used as the CRD group name in the GVK.
	// It is computed from information in the plugin trait, using plugin.id unless groupName is specified.
	// The length of the computed group + the length of the name (plus 1) cannot exceed 63 characters for a valid CRD.
	// This length restriction is checked via _computedGroupKind
	fullGroup: _computedGroups[0] & =~"^([a-z][a-z0-9-.]{0,61}[a-z0-9])$"

	_computedGroupKinds: [
		for x in S.kinds {
			let computed = S.machineName+"."+group & =~"^([a-z][a-z0-9-.]{0,63}[a-z0-9])$"
			if computed =~ "^([a-z][a-z0-9-.]{0,63}[a-z0-9])$" {
				computed
			}
		},
	]
}

//
// LEGACY TYPES USED FOR PARSING
// These types are used for parsing "old style" kinds which contain versions (rather than versions which contain kinds).
// These should not be updated, and will be removed in a future release
//

KindOld: S={
	kind:  =~"^([A-Z][a-zA-Z0-9-]{0,61}[a-zA-Z0-9])$"
	group: =~"^([a-z][a-z0-9-.]{0,61}[a-z0-9])$"
	// manifestGroup is a group shortname used for package naming in codegen
	// TODO: remove this when all jenny pipelines use the manifest, or keep around for convenience?
	manifestGroup: string
	current:       string
	// scope determines whether resources of this kind exist globally ("Cluster") or
	// within Kubernetes namespaces.
	scope: "Cluster" | *"Namespaced"
	// validation determines whether there is code-based validation for this kind.
	validation: #AdmissionCapability | *{
		operations: []
	}
	// mutation determines whether there is code-based mutation for this kind.
	mutation: #AdmissionCapability | *{
		operations: []
	}
	// conversion determines whether there is code-based conversion for this kind.
	conversion: bool | *false
	// conversionWebhookProps is a temporary way of specifying the service webhook information
	// which will be migrated away from once manifests are used in the codegen pipeline
	conversionWebhookProps: {
		url: string | *""
	}
	versions: {
		[V=string]: {
			// Version must be the key in the map, but is pulled into the value of the map for ease-of-access when dealing with the resulting value
			version: V
			schema:  _
			// served indicates whether this version is served by the API server
			served: bool | *true
			// codegen contains properties specific to generating code using tooling
			codegen: {
				ts: {
					enabled: bool | *S.codegen.ts.enabled
					config: {
						importsMap: {
							[string]: string
						} | *S.codegen.ts.config.importsMap
						enumsAsUnionTypes: bool | *S.codegen.ts.config.enumsAsUnionTypes
					} | *S.codegen.ts.config
				}
				go: {
					enabled: bool | *S.codegen.go.enabled
					config: {} | *S.codegen.go.config
				}
			}
			// seledtableFields is a list of additional fields which can be used in kubernetes field selectors for this version.
			// Fields must be from the root of the schema, i.e. 'spec.foo', and have a string type.
			// Fields cannot include custom metadata (TODO: check if we can use annotations for field selectors)
			selectableFields: [...string] & list.MaxItems(8)
			validation: #AdmissionCapability | *S.validation
			mutation:   #AdmissionCapability | *S.mutation
			// additionalPrinterColumns is a list of additional columns to be printed in kubectl output
			additionalPrinterColumns?: [...#AdditionalPrinterColumns]
			// routes is a map of path patterns to custom routes for this version.
			routes?: #CustomRouteCapability
		}
	}
	machineName:       strings.ToLower(strings.Replace(S.kind, "-", "_", -1))
	pluralName:        =~"^([A-Z][a-zA-Z0-9-]{0,61}[a-zA-Z])$" | *(S.kind + "s")
	pluralMachineName: strings.ToLower(strings.Replace(S.pluralName, "-", "_", -1))
	// codegen contains properties specific to generating code using tooling. At the root level of the kind, it sets
	// the defaults for the `codegen` field in all entries in `versions`.
	// Valus set in `versions[x]: codegen` will overwrite the value set here.
	codegen: {
		// ts is the section for TypeScript codegen
		ts: {
			// enabled indicates whether front-end TypeScript code should be generated for this kind's schema
			enabled: bool | *true
			// config is code generation configuration specific to TypeScript.
			// Currently, these config options are passed directly to grafana/cog when generating TypeScript
			config: {
				// importsMap associates package names to their import path.
				importsMap: {
					[string]: string
				}
				// enumsAsUnionTypes generates enums as a union of values instead of using
				// an actual `enum` declaration.
				// If EnumsAsUnionTypes is false, an enum will be generated as:
				// "`ts
				// enum Direction {
				//   Up = "up",
				//   Down = "down",
				//   Left = "left",
				//   Right = "right",
				// }
				// "`
				// If EnumsAsUnionTypes is true, the same enum will be generated as:
				// "`ts
				// type Direction = "up" | "down" | "left" | "right";
				// "`
				enumsAsUnionTypes: bool | *false
			}
		}
		// go is the section for go codegen
		go: {
			// enabled indicates whether back-end Go code should be generated for this kind's schema
			enabled: bool | *true
			config: {
			}
		}
	}

	_computedGroupKind: S.machineName+"."+group & =~"^([a-z][a-z0-9-.]{0,63}[a-z0-9])$"
}

ManifestOld: S={
	appName: =~"^([a-z][a-z0-9-]*[a-z0-9])$"
	group:   strings.ToLower(strings.Replace(S.appName, "-", "", -1))
	kinds: [...{
		group:         S.fullGroup
		manifestGroup: S.group
	} & KindOld]
	extraPermissions: {
		accessKinds: [...#AccessKind]
	}

	// operatorURL is the HTTPS URL of your operator, including port if non-standard (443).
	// If you do not deploy an operator, or if your operator does not expose an HTTPS server for webhooks, this can be omitted.
	// This is used to construct validation, mutations, or conversion webhooks for your deployment.
	operatorURL?: string

	// groupOverride is used to override the auto-generated group of "<group>.ext.grafana.app"
	// if present, this value is used for the full group instead.
	// groupOverride must have at least two parts (i.e. 'foo.bar'), but can be longer.
	// The length of fullGroup + kind name (for each kind) cannot exceed 62 characters
	groupOverride?: =~"^([a-z][a-z0-9-.]{0,48}[a-z0-9])\\.([a-z][a-z0-9-]{0,48}[a-z0-9])$"

	// _computedGroups is a list of groups computed from information in the plugin trait.
	// The first element is always the "most correct" one to use.
	// This field could be inlined into `group`, but is separate for clarity.
	_computedGroups: [
		if S.groupOverride != _|_ {
			strings.ToLower(S.groupOverride)
		},
		strings.ToLower(strings.Replace(S.group, "_", "-", -1)) + ".ext.grafana.com", // TODO: change to ext.grafana.app?
	]

	// fullGroup is used as the CRD group name in the GVK.
	// It is computed from information in the plugin trait, using plugin.id unless groupName is specified.
	// The length of the computed group + the length of the name (plus 1) cannot exceed 63 characters for a valid CRD.
	// This length restriction is checked via _computedGroupKind
	fullGroup: _computedGroups[0] & =~"^([a-z][a-z0-9-.]{0,61}[a-z0-9])$"

	_computedGroupKinds: [
		for x in S.kinds {
			let computed = S.machineName+"."+group & =~"^([a-z][a-z0-9-.]{0,63}[a-z0-9])$"
			if computed =~ "^([a-z][a-z0-9-.]{0,63}[a-z0-9])$" {
				computed
			}
		},
	]
}