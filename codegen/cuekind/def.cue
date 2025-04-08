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
	status: _

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
			selectableFields: [...string]
			validation: #AdmissionCapability | *S.validation
			mutation:   #AdmissionCapability | *S.mutation
			// additionalPrinterColumns is a list of additional columns to be printed in kubectl output
			additionalPrinterColumns?: [...#AdditionalPrinterColumns]
			// customRoutes is a map of of path patterns to custom routes for this version.
			customRoutes?: {
				[string]: #PathProps
			}
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
				// allowMarshalEmptyDisjunctions determines whether to allow marshaling empty disjunctions.
				// If true, empty disjunctions will be marshaled as `null` instead of returning an error.
				allowMarshalEmptyDisjunctions: bool | *true
			}
		}
	}

	_computedGroupKind: S.machineName+"."+group & =~"^([a-z][a-z0-9-.]{0,63}[a-z0-9])$"
}

#AccessKind: {
	group:    string
	resource: string
	actions: [...string]
}

Manifest: S={
	appName: =~"^([a-z][a-z0-9-]*[a-z0-9])$"
	group:   strings.ToLower(strings.Replace(S.appName, "-", "", -1))
	kinds: [...{
		group:         S.fullGroup
		manifestGroup: S.group
	} & Kind]
	extraPermissions: {
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

#PathProps: {
	// summary is a short summary of what the operation does
	summary?: string
	// description is a verbose explanation of the operation behavior
	description?: string
	// get defines a GET operation
	get?: #Operation
	// put defines a PUT operation
	put?: #Operation
	// post defines a POST operation
	post?: #Operation
	// delete defines a DELETE operation
	delete?: #Operation
	// options defines an OPTIONS operation
	options?: #Operation
	// head defines a HEAD operation
	head?: #Operation
	// patch defines a PATCH operation
	patch?: #Operation
	// trace defines a TRACE operation
	trace?: #Operation
	// servers is an alternative server array to service this operation
	servers?: [...#Server]
	// parameters is a list of parameters that are applicable for this operation
	parameters?: [...#Parameter]
}

#Operation: {
	// tags is a list of tags for API documentation control
	tags?: [...string]
	// summary is a short summary of what the operation does
	summary?: string
	// description is a verbose explanation of the operation behavior
	description?: string
	// externalDocs is additional external documentation for this operation
	externalDocs?: #ExternalDocumentation
	// operationId is a unique identifier for this operation
	operationId?: string
	// parameters is a list of parameters that are applicable for this operation
	parameters?: [...#Parameter]
	// requestBody is the request body applicable for this operation
	requestBody?: #RequestBody
	// responses is a map of possible responses as they are returned from executing this operation
	responses?: {
		[string]: #Response
	}
	// deprecated declares this operation to be deprecated
	deprecated?: bool
	// security is a declaration of which security mechanisms can be used for this operation
	security?: [...#SecurityRequirement]
	// servers is an alternative server array to service this operation
	servers?: [...#Server]
}

#Parameter: {
	// name is the name of the parameter
	name: string
	// in is the location of the parameter
	in: "query" | "header" | "path" | "cookie"
	// description is a brief description of the parameter
	description?: string
	// required determines whether this parameter is mandatory
	required?: bool
	// deprecated declares this parameter to be deprecated
	deprecated?: bool
	// allowEmptyValue sets the ability to pass empty-valued parameters
	allowEmptyValue?: bool
	// style defines how the parameter value will be serialized depending on the type of the parameter value
	style?: string
	// explode when true, parameter values of type array or object generate separate parameters for each value of the array or key-value pair of the map
	explode?: bool
	// allowReserved determines whether the parameter value should allow reserved characters
	allowReserved?: bool
	// schema defines the type used for the parameter
	schema?: #Schema
	// example is an example of the parameter's potential value
	example?: _
	// examples are examples of the parameter's potential value
	examples?: {
		[string]: #Example
	}
	// content is a map containing the representations for the parameter
	content?: {
		[string]: #MediaType
	}
}

#RequestBody: {
	// description is a brief description of the request body
	description?: string
	// content is the content of the request body
	content: {
		[string]: #MediaType
	}
	// required determines if the request body is required in the request
	required?: bool
}

#Responses: {
	// default is the documentation of responses other than the ones declared for specific HTTP response codes
	default?: #Response
	// responses is a map of any HTTP status code to the response definition
	responses?: {
		[string]: #Response
	}
}

#Response: {
	// description is a short description of the response
	description: string
	// headers is a map of possible headers that are sent with the response
	headers?: {
		[string]: #Header
	}
	// content is a map containing descriptions of potential response payloads
	content?: {
		[string]: #MediaType
	}
	// links is a map of operations links that can be followed from the response
	links?: {
		[string]: #Link
	}
}

#MediaType: {
	// schema defines the schema defining the type used for the request body
	schema?: #Schema
	// example is an example of the media type
	example?: _
	// examples are examples of the media type
	examples?: {
		[string]: #Example
	}
	// encoding is a map between a property name and its encoding information
	encoding?: {
		[string]: #Encoding
	}
}

#Schema: {
	// type is the type of the schema
	type?: string
	// format is the format of the schema
	format?: string
	// description is a brief description of the schema
	description?: string
	// title is the title of the schema
	title?: string
	// default is the default value of the schema
	default?: _
	// enum specifies the list of allowed values
	enum?: [..._]
	// multipleOf is the multiple of the schema
	multipleOf?: number
	// maximum is the maximum value of the schema
	maximum?: number
	// exclusiveMaximum is the exclusive maximum value of the schema
	exclusiveMaximum?: bool
	// minimum is the minimum value of the schema
	minimum?: number
	// exclusiveMinimum is the exclusive minimum value of the schema
	exclusiveMinimum?: bool
	// maxLength is the maximum length of the schema
	maxLength?: int
	// minLength is the minimum length of the schema
	minLength?: int
	// pattern is the pattern of the schema
	pattern?: string
	// maxItems is the maximum number of items in the schema
	maxItems?: int
	// minItems is the minimum number of items in the schema
	minItems?: int
	// uniqueItems determines if the schema items must be unique
	uniqueItems?: bool
	// maxProperties is the maximum number of properties in the schema
	maxProperties?: int
	// minProperties is the minimum number of properties in the schema
	minProperties?: int
	// required is a list of required properties in the schema
	required?: [...string]
	// properties is a map of properties in the schema
	properties?: {
		[string]: #Schema
	}
	// additionalProperties is the additional properties of the schema
	additionalProperties?: #Schema | bool
	// items is the items of the schema
	items?: #Schema
	// allOf is a list of schemas that must all be valid
	allOf?: [...#Schema]
	// oneOf is a list of schemas where exactly one must be valid
	oneOf?: [...#Schema]
	// anyOf is a list of schemas where at least one must be valid
	anyOf?: [...#Schema]
	// not is a schema that must not be valid
	not?: #Schema
	// nullable determines if the schema can be null
	nullable?: bool
	// discriminator is the discriminator for the schema
	discriminator?: #Discriminator
	// externalDocs is external documentation for the schema
	externalDocs?: #ExternalDocumentation
	// deprecated declares this schema to be deprecated
	deprecated?: bool
	// xml is XML-specific attributes for the schema
	xml?: #XML
}

#Example: {
	// summary is a short summary of the example
	summary?: string
	// description is a description of the example
	description?: string
	// value is the value of the example
	value?: _
	// externalValue is a URL that points to the literal example
	externalValue?: string
}

#Encoding: {
	// contentType is the Content-Type for encoding a specific property
	contentType?: string
	// headers is a map allowing additional information to be included as headers
	headers?: {
		[string]: #Header
	}
	// style describes how the parameter value will be serialized depending on the type of the parameter value
	style?: string
	// explode when true, property values of type array or object generate separate parameters for each value of the array or key-value pair of the map
	explode?: bool
	// allowReserved determines whether the parameter value should allow reserved characters
	allowReserved?: bool
}

#Header: {
	// description is a brief description of the header
	description?: string
	// required determines if the header is required
	required?: bool
	// deprecated declares this header to be deprecated
	deprecated?: bool
	// allowEmptyValue sets the ability to pass empty-valued headers
	allowEmptyValue?: bool
	// style defines how the header value will be serialized depending on the type of the header value
	style?: string
	// explode when true, header values of type array or object generate separate headers for each value of the array or key-value pair of the map
	explode?: bool
	// allowReserved determines whether the header value should allow reserved characters
	allowReserved?: bool
	// schema defines the type used for the header
	schema?: #Schema
	// example is an example of the header's potential value
	example?: _
	// examples are examples of the header's potential value
	examples?: {
		[string]: #Example
	}
	// content is a map containing the representations for the header
	content?: {
		[string]: #MediaType
	}
}

#Link: {
	// operationRef is a relative or absolute reference to an OAS operation
	operationRef?: string
	// operationId is the name of an existing, resolvable operation, as defined with a unique operationId
	operationId?: string
	// parameters is a map representing parameters to pass to an operation as specified with operationId or identified via operationRef
	parameters?: {
		[string]: _
	}
	// requestBody is a literal value or expression to use as a request body when calling the target operation
	requestBody?: _
	// description is a description of the link
	description?: string
	// server is a server object to be used by the target operation
	server?: #Server
}

#Server: {
	// url is the URL to the target host
	url: string
	// description is an optional string describing the host designated by the URL
	description?: string
	// variables is a map between a variable name and its value
	variables?: {
		[string]: #ServerVariable
	}
}

#ServerVariable: {
	// enum is an enumeration of string values to be used if the substitution options are from a limited set
	enum?: [...string]
	// default is the default value to use for substitution
	default: string
	// description is a description for the server variable
	description?: string
}

#SecurityRequirement: {
	[string]: [...string]
}

#ExternalDocumentation: {
	// description is a short description of the target documentation
	description?: string
	// url is the URL for the target documentation
	url: string
}

#Discriminator: {
	// propertyName is the name of the property in the payload that will hold the discriminator value
	propertyName: string
	// mapping is an object to hold mappings between payload values and schema names or references
	mapping?: {
		[string]: string
	}
}

#XML: {
	// name is the name of the XML element
	name?: string
	// namespace is the namespace of the XML element
	namespace?: string
	// prefix is the prefix of the XML element
	prefix?: string
	// attribute determines if the property should be treated as an attribute
	attribute?: bool
	// wrapped determines if the property should be wrapped in an array
	wrapped?: bool
}
