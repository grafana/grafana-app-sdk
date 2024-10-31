package cuekind

import (
	"strings"
	"struct"
	"time"
)

// _kubeObjectMetadata is metadata found in a kubernetes object's metadata field.
// It is not exhaustive and only includes fields which may be relevant to a kind's implementation,
// As it is also intended to be generic enough to function with any API Server.
_kubeObjectMetadata: {
    uid: string
    creationTimestamp: string & time.Time
    deletionTimestamp?: string & time.Time
    finalizers: [...string]
    resourceVersion: string
	generation: int64
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
		createdBy: string
		updatedBy: string
	} & {
		// All extensions to this metadata need to have string values (for APIServer encoding-to-annotations purposes)
		// Can't use this as it's not yet enforced CUE:
		//...string
		// Have to do this gnarly regex instead
		[!~"^(uid|creationTimestamp|deletionTimestamp|finalizers|resourceVersion|generation|labels|updateTimestamp|createdBy|updatedBy|extraFields)$"]: string
	}
	spec: _
	status: {
		[string]: _
	}

	// cuetsy is not happy creating spec with the MinFields constraint directly
	_specIsNonEmpty: spec & struct.MinFields(0)
}

#AdmissionCapability: {
	operations: [...string]
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
	kind: =~"^([A-Z][a-zA-Z0-9-]{0,61}[a-zA-Z0-9])$"
	group: =~"^([a-z][a-z0-9-]*[a-z0-9])$"
	current: string
	// apiResource contains properties specific to converting this kind to a Kubernetes API Server Resource.
	// apiResource is an optional trait that imposes some restrictions on `schema`, `group`, and `version`.
	// If not present, the kind cannot be assumed to be convertable to a kubernetes API server resource.
	apiResource?: {
		// groupOverride is used to override the auto-generated group of "<group>.ext.grafana.com"
		// if present, this value is used for the CRD group instead.
		// groupOverride must have at least two parts (i.e. 'foo.bar'), but can be longer.
		// The length of groupOverride + kind name cannot exceed 62 characters
		groupOverride?: =~"^([a-z][a-z0-9-.]{0,48}[a-z0-9])\\.([a-z][a-z0-9-]{0,48}[a-z0-9])$"

		// _computedGroups is a list of groups computed from information in the plugin trait.
		// The first element is always the "most correct" one to use.
		// This field could be inlined into `group`, but is separate for clarity.
		_computedGroups: [
			if S.apiResource.groupOverride != _|_ {
				strings.ToLower(S.apiResource.groupOverride),
			},
			strings.ToLower(strings.Replace(S.group, "_","-",-1)) + ".ext.grafana.com"
		]

		// group is used as the CRD group name in the GVK.
		// It is computed from information in the plugin trait, using plugin.id unless groupName is specified.
		// The length of the computed group + the length of the name (plus 1) cannot exceed 63 characters for a valid CRD.
		// This length restriction is checked via _computedGroupKind
		group: _computedGroups[0] & =~"^([a-z][a-z0-9-.]{0,61}[a-z0-9])$"

		// _computedGroupKind checks the validity of the CRD kind + group
		_computedGroupKind: S.machineName + "." + group & =~"^([a-z][a-z0-9-.]{0,63}[a-z0-9])$"

		// scope determines whether resources of this kind exist globally ("Cluster") or
		// within Kubernetes namespaces.
		scope: "Cluster" | *"Namespaced"
		// validation determines whether there is code-based validation for this kind. Used for generating the manifest.
		validation: #AdmissionCapability | *{
			operations: []
		}
		// mutation determines whether there is code-based mutation for this kind. Used for generating the manifest.
		mutation: #AdmissionCapability | *{
			operations: []
		}
		// conversion determines whether there is code-based conversion for this kind. Used for generating the manifest.
		conversion: bool | *false
	}
	// isCRD is true if the `crd` trait is present in the kind.
	isAPIResource: apiResource != _|_
	versions: {
		[V=string]: {
			// Version must be the key in the map, but is pulled into the value of the map for ease-of-access when dealing with the resulting value
			version: V
			schema: _
			// served indicates whether this version is served by the API server
			served: bool | *true
			// codegen contains properties specific to generating code using tooling
			codegen: {
				// frontend indicates whether front-end TypeScript code should be generated for this kind's schema
				frontend: bool | *S.codegen.frontend
				// backend indicates whether back-end Go code should be generated for this kind's schema
				backend: bool | *S.codegen.backend
			}
			// seledtableFields is a list of additional fields which can be used in kubernetes field selectors for this version.
			// Fields must be from the root of the schema, i.e. 'spec.foo', and have a string type.
			// Fields cannot include custom metadata (TODO: check if we can use annotations for field selectors)
			selectableFields: [...string]
			validation: #AdmissionCapability | *S.apiResource.validation
			mutation: #AdmissionCapability | *S.apiResource.mutation
			// additionalPrinterColumns is a list of additional columns to be printed in kubectl output
			additionalPrinterColumns?: [...#AdditionalPrinterColumns]
		}
	}
	machineName: strings.ToLower(strings.Replace(S.kind, "-", "_", -1))
	pluralName: =~"^([A-Z][a-zA-Z0-9-]{0,61}[a-zA-Z])$" | *(S.kind + "s")
	pluralMachineName: strings.ToLower(strings.Replace(S.pluralName, "-", "_", -1))
	// codegen contains properties specific to generating code using tooling. At the root level of the kind, it sets
	// the defaults for the `codegen` field in all entries in `versions`. 
	// Valus set in `versions[x]: codegen` will overwrite the value set here.
	codegen: {
		// frontend indicates whether front-end TypeScript code should be generated for this kind's schema
		frontend: bool | *true
		// backend indicates whether back-end Go code should be generated for this kind's schema
		backend: bool | *true
	}
}

#AccessKind: {
	group: string
	resource: string
	actions: [...string]
}

Manifest: S={
	appName: string
	group: =~"^([a-z][a-z0-9-]*[a-z0-9])$"
	kinds: [...{
		group: S.group
	} & Kind]
	permissions: {
		accessKinds: [...#AccessKind]
	}

	// groupOverride is used to override the auto-generated group of "<group>.ext.grafana.app"
	// if present, this value is used for the full group instead.
	// groupOverride must have at least two parts (i.e. 'foo.bar'), but can be longer.
	// The length of fullGroup + kind name (for each kind) cannot exceed 62 characters
	groupOverride?: =~"^([a-z][a-z0-9-.]{0,48}[a-z0-9])\\.([a-z][a-z0-9-]{0,48}[a-z0-9])$"

	// _computedGroups is a list of groups computed from information in the plugin trait.
	// The first element is always the "most correct" one to use.
	// This field could be inlined into `group`, but is separate for clarity.
	_computedGroups: [
		if S.apiResource.groupOverride != _|_ {
			strings.ToLower(S.apiResource.groupOverride),
		},
		strings.ToLower(strings.Replace(S.group, "_","-",-1)) + ".ext.grafana.app"
	]

	// fullGroup is used as the CRD group name in the GVK.
	// It is computed from information in the plugin trait, using plugin.id unless groupName is specified.
	// The length of the computed group + the length of the name (plus 1) cannot exceed 63 characters for a valid CRD.
	// This length restriction is checked via _computedGroupKind
	fullGroup: _computedGroups[0] & =~"^([a-z][a-z0-9-.]{0,61}[a-z0-9])$"

	_computedGroupKinds: [
		for x in S.kinds {
			let computed = S.machineName + "." + group & =~"^([a-z][a-z0-9-.]{0,63}[a-z0-9])$"
			if computed =~"^([a-z][a-z0-9-.]{0,63}[a-z0-9])$" {
				computed
			}
		}
	]
}