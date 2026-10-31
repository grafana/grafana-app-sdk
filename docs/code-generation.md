# Code Generation

Code generation done by the grafana-app-sdk can be broadly split into two buckets: what we call **kind code generation** which is run to generate code from your CUE kinds which can be used in your app, and **project component generation**, which is a run-once kind of codegen that scaffolds your project with boilerplate code that you can then alter as you see fit.

**code generation** is done with the `generate` command in the CLI, and **project component generation** is done with the `project component add` command in the CLI (also note that `project init` is a special case of codegen that scaffolds your initial project with opinionated defaults). The general workflow is that component generation is run only once, while kind code generation is run whenever you update or add a kind, or whenever you update your version of the grafana-app-sdk (to ensure that your generated code works with the new library version).

## Kind Code Generation

Code generation turns kinds written in CUE into go and TypeScript code which can be used to write your app logic. 
A full breakdown on writing CUE kinds and using them with the CLI's code generation can be found in the [Writing Kinds](./custom-kinds/writing-kinds.md) document.

Kind codegen uses `grafana-app-sdk generate` as its base command. Generation settings are supplied via `config.cue`. The following is an example `config.cue` for kind code generation:
```cue
config: {
	definitions: {
		manifestSchemas: true
		manifestVersion: "v1alpha2"
		path:            "definitions"
		encoding:        "json"
	}

	kinds: {
		grouping: "kind"
	}

	codegen: {
		goEnabled:                      true
		goGenPath:                      "pkg/generated/"
		tsGenPath:                      "plugin/src/generated/"
		enableK8sPostProcessing:        false
		enableOperatorStatusGeneration: true
	}
}
```
`grafana-app-sdk generate` scans the `source` directory for CUE files, and parses all top-level fields in all present CUE files as CUE kinds. If kind validation encounters any errors, no files will be written, and the validation error(s) will be printed out. On successful generation: 
* kind go code will be written to the path in `config.codegen.goGenPath`, with a package for each unique kind-version combination (unless `config.codegen.goEnabled` is `false`)
* kind TypeScript code will be written to the path in `config.codegen.tsGenPath`, with a folder for each unique kind-version combination
* kind CRD files and app manifest will be written to `config.definitions.path`, encoded according to `config.definitions.encoding`

By default the app manifest is named `<appName>-manifest.<encoding>`. Set `config.definitions.manifestFileName` to write it under a specific name instead (still relative to `config.definitions.path`):
```cue
config: {
	definitions: {
		path:             "definitions"
		manifestFileName: "my-app.json"
	}
}
```
Because one manifest is generated per entry in `config.manifestSelectors`, `manifestFileName` can only be used when a single manifest is generated — otherwise every manifest would be written to the same file, so codegen errors out instead. Note that `manifestFileName` sets only the filename; the file's contents are still encoded according to `config.definitions.encoding`.

Setting `config.codegen.goEnabled` to `false` disables go code generation entirely, for every kind. This is intended for frontend-only apps: no go files are written, and neither a `go.mod` nor the `go` binary is required to run codegen. TypeScript, CRD, and app manifest JSON/YAML generation are unaffected. To disable go codegen for an individual kind or version instead of the whole project, see [Toggling TypeScript/Go Codegen](./custom-kinds/writing-kinds.md#toggling-typescriptgo-codegen).

> [!IMPORTANT]
> Because the interfaces that the grafana-app-sdk libraries use can change, be sure to run kind code generation using a version of the `grafana-app-sdk` CLI that matches the version of the dependency you use in your project. Whenever you update the dependency, make sure you re-run the kind code generation as well.

Please see [Writing Kinds](./custom-kinds/writing-kinds.md) for a more detailed look at kind code generation from CUE.

## Project Component Generation

Project component generation is used to add boilerplate code for a "component" of your app. Components understood by the SDK are:
* `frontend` - a frontend plugin for your app, written in TypeScript. This is deployed in grafana as a standard app plugin.
* `operator` - a standalone operator, written in go.

Multiple components can be specified in the same command. The full syntax is:
```
grafana-app-sdk project component add <list of space-separated components> [-s|--source=kinds]
```

A list of valid components can also be found by running 
```
grafana-app-sdk project component add
```
Without any components to add

### frontend

The frontend component generates TypeScript code and configuration for a grafana plugin in `plugin`, similar to the output of the npm [grafana/create-plugin](https://www.npmjs.com/package/@grafana/create-plugin) tool (if you would like to use `create-plugin` instead, see the [Get started](https://grafana.com/developers/plugin-tools/) docs, and create an app plugin).

> [!NOTE]
> The `backend` component, which generated a `pkg/plugin` proxy to the API server, was removed in v0.61.
> The frontend can call the API server directly. See the [v0.61 migration guide](./migrations/v0.61.md).

### operator

The operator component generates a boilerplate watcher for each kind in `source` in `pkg/watchers/`. The rest of the operator code is generated in `cmd/operator`, and includes configuration, telemetry, a `main` file that sets up an operator using the `simple` package, and a Dockerfile for building a deploying a docker image for your operator. The generated boilerplate code works with the `operator` targets in the `Makefile` generated by `grafana-app-sdk project init`.

> [!NOTE]
> The `project` command also has a `kind` target, which can be used to generate a boilerplate kind, with
> ```
> grafana-app-sdk project kind add <kind name> [-s|--source=kinds]
> ```
> This is not a component command, but is useful for quickly generating a CUE kind in your `source` directory.

## Project Initialization

Project initialization is a special case of code generation used to initialize a new app-sdk project. It is not required to work with the app-sdk, but exists as a convenient way of setting up a project with the default workflow of the grafana-app-sdk. The syntax is:
```
grafana-app-sdk project init <project go module name>
```
This sets up your project with a go module, a `kinds` directory with a CUE module, a `Makefile` with some sample targets, and a `local` directory that can be used with `grafana-app-sdk project local` commands.

## Documenting custom routes

A custom route declared in CUE identifies itself with an `operationId`, and can carry the
documentation that ends up in the OpenAPI the manifest publishes, so clients generated from it are
readable:

```cue
routes: namespaced: "/update/{group}/{kind}/{id}": PUT: {
    operationId: "addStar"
    summary:     "Add a starred item"
    description: "Adds the item to the stars of the object."
    request: parameters: [
        {name: "group", in: "path", description: "API group of the item", example: "dashboard.grafana.app"},
        {name: "kind", in: "path", description: "Kind of the item", example: "Dashboard"},
        {name: "id", in: "path", description: "Name of the item"},
        {name: "reason", in: "query", description: "Why the item is starred", schema: string | *"reference"},
    ]
    response: {message: string}
}
```

- `operationId` is required. It is the `operationId` of the OpenAPI operation, so say what the
  route does, and it must be unique among the routes of a version, on its kinds and on the version
  itself, because OpenAPI does not allow two in a document. It also names the generated Go types and
  client method (`AddStar` here), so it has to be an identifier.
- `name` is the deprecated spelling of `operationId`, kept so existing routes keep working. A route
  declares one or the other, and `operationId` wins when both are there. `name` has to start with a
  Kubernetes verb (`get`, `create`, `delete`, ...), which `operationId` does not.
- `summary` and `description` describe the operation.
- `request.parameters` lists the parameters of the route, as in OpenAPI: each one has a `name` and
  says where it is with `in` (`path`, `query`, `header` or `cookie`). A parameter is identified by
  its name and `in`, so two cannot share both. The other fields are `description`, `required`,
  `deprecated`, an `example` and a `schema`, which defaults to a string. An `example` has to be a
  value of the schema's type.
  - A path parameter has to be a parameter in the path, and is always required. Naming one that is
    not in the path is an error.
  - A query parameter is defined here: `schema` is its type, and `required` is whether it has to be
    sent (false by default). They are also what the generated Go client takes for the route.
    `request.query`, a CUE struct of the fields, is the deprecated spelling. A route declares its
    query parameters in one or the other, not both.
  - A header or cookie parameter is documented in the OpenAPI only. It is not part of the generated
    client or the request types yet.

For anything beyond this, such as responses other than the default, import or declare the OpenAPI
itself, as below.

### Returning the resource of a kind

A route that returns the same resource the standard Kubernetes calls return for a kind, such as a
route that finds the latest or the starred one, names the kind with `responseKind` instead of
defining a `response`:

```cue
versions: v1: {
    kinds: [star]  // a kind named Star
    routes: namespaced: "/latest": GET: {
        operationId:  "getLatestStar"
        summary:      "Get the most recently starred item"
        responseKind: "Star"
    }
}
```

- The OpenAPI response references the kind's resource, a `Star` component of the version with the
  `apiVersion`, `kind`, `metadata`, `spec` and `status` of the kind, rather than a new schema. What
  it references of the kind's schema (such as its `spec`) is a component named for the kind, like
  `Star.spec`.
- No response type is generated for the route. The generated client method returns the kind's own
  generated Go type, here `(*Star, error)`, so what it returns can be used wherever the kind's
  objects are.
- `responseKind` has to be a kind of the same version, which generates Go code, and a route cannot
  declare both it and a `response`. Its `responseMetadata` is not used, since the kind's resource
  has its own.
- When generating code by kind, a route can only return its own kind, because the types of other
  kinds are in other packages. A version-level route cannot use `responseKind` then.

## Adding routes from a saved OpenAPI document

Manifest generation with `definitions.manifestVersion: "v1alpha2"` (the default)
can combine CUE with an OpenAPI 3.0 document for each API version.
Place `openapi.v1.json`, `openapi.v1.yaml`, or `openapi.v1.yml` beside
`manifest.cue` in the CUE source directory. Repeat for other versions declared in
the manifest, such as `openapi.v2.json`. Existing CUE-only projects need no changes.

To use a different filename, specify it on the version:

```cue
manifest: {
    appName: "example"
    versions: v1: {
        importOpenAPIFile: "saved-api.yaml"
        kinds: []
    }
}
```

Paths are relative to the CUE source directory, including when using `--source`.
An explicit `importOpenAPIFile` value takes precedence over automatic discovery. If multiple
conventional filenames exist for one version, select one explicitly.

You can also declare OpenAPI paths and components inline on a version. The
`openapi` block uses the same structure as the `paths` and `components` sections
of an OpenAPI 3.0 document, written as CUE:

```cue
versions: v1: {
    kinds: []
    openapi: {
        paths: {
            "/namespaces/{namespace}/reports/{report}": {
                get: {
                    operationId: "getReport"
                    summary:     "Fetch a single report"
                    parameters: [{
                        name:     "report"
                        in:       "path"
                        required: true
                        schema: type: "string"
                    }]
                    responses: {
                        "200": {
                            description: "The requested report"
                            content: {
                                "application/json": {
                                    schema: {
                                        "$ref": "#/components/schemas/Report"
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
        components: {
            schemas: {
                Report: {
                    type: "object"
                    required: ["title"]
                    properties: {
                        title: type: "string"
                        count: {
                            type:   "integer"
                            format: "int64"
                        }
                    }
                }
            }
        }
    }
}
```

This is equivalent to the following OpenAPI YAML (the `{namespace}` path
parameter is added automatically):

```yaml
paths:
  /namespaces/{namespace}/reports/{report}:
    get:
      operationId: getReport
      summary: Fetch a single report
      parameters:
        - name: report
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: The requested report
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Report'
components:
  schemas:
    Report:
      type: object
      required: [title]
      properties:
        title:
          type: string
        count:
          type: integer
          format: int64
```

Inline OpenAPI overrides matching CUE route operations. If a saved document is
also loaded, it overrides matching inline operations and schemas. Other
operations remain. Each OpenAPI source must resolve its own local references.
Shared path-level parameters are copied into each operation before merging;
operation-level parameters override shared parameters with the same name and
location. This preserves the parameters of retained operations and makes them
available to the legacy route installer.

All custom route schemas for a version share one `components.schemas` map. If two
kinds (or a kind and the version-level routes) define a schema with the same name
but different contents, generation fails; move the shared type to the inline
`openapi.components.schemas` section and reference it from each route.
Imported subresource schemas must also avoid conflicting with the kind's resource
schemas, including `spec` and the root kind name. Identical schemas can be shared;
different schemas must use different names.

For example, `saved-api.yaml` can introduce a route and a response type absent
from CUE:

```yaml
openapi: 3.0.3
info:
  title: Example API
  version: v1
  x-grafana-api-group: example.ext.grafana.app
paths:
  /namespaces/{namespace}/reports:
    get:
      operationId: getReports
      responses:
        '200':
          description: A report
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Report'
components:
  schemas:
    Report:
      type: object
      required: [title]
      properties:
        title:
          type: string
```

The generated JSON/YAML manifest and embedded Go manifest include these custom
routes and schemas. They do not cause Go or TypeScript types, clients, or handlers
to be generated. Resource kinds and their metadata continue to be declared in CUE;
OpenAPI components describe the custom routes' request and response types.

Route paths are relative to `/apis/<manifest-group>/<version>`. The processing
engine derives this prefix from the CUE manifest and the version importing the
document, so it does not need to appear in each path. `info.version` and the
optional `info.x-grafana-api-group` document this context; they do not override CUE.

## Examples & Testing

Code generation for both kinds and project components is done as part of the [issue tracker tutorial](./tutorials/issue-tracker/README.md) ([kind code generation](./tutorials/issue-tracker/03-generate-kind-code.md), [project component generation](./tutorials/issue-tracker/04-boilerplate.md)).

Automated testing of kind code generation is done using the files in [codegen/cuekind/testing/](../codegen/cuekind/testing/), with generated files compared against [codegen/testing/golden_generated](../codegen/testing/golden_generated/).

The combined CUE/OpenAPI integration manifest has a JSON snapshot in
[codegen/cuekind/testing/golden](../codegen/cuekind/testing/golden/). It is
checked by `TestManifestGenerator_IntegrationOpenAPI` (which also decodes and
verifies the YAML output) and is intentionally outside the CLI comparison fixtures.
To update it after an intentional output change:

```sh
UPDATE_INTEGRATION_GOLDEN=1 go test ./codegen/cuekind -run '^TestManifestGenerator_IntegrationOpenAPI$' -count=1
```
