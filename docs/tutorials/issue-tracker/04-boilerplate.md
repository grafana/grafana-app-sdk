# Generating Boilerplate

Since this is a fresh project, we can take advantage of the CLI's tooling to set up boilerplate code for us which we can then extend on. Note that this is not strictly necessary for writing an application (whereas running the CUE codegen is something you'll likely want for every project), but it makes initial project bootstrapping simpler, and will help us move along here faster. If you decide in future projects you want to handle your routing, storage, or front-end framework differently, you can eschew some or all of the things laid out in this section.

## The `project add` Command

Earlier, we used the CLI's `project` command with `project init`, initializing our project with some very basic stuff. Now, we can again use the `project` command, this time to add boilerplate components to our app. These are added using the `project add` command, with the name of one or more components you wish to add to the project. To see the list of possible components, you can run it sans arguments, like so:
```shell
$ grafana-app-sdk project add
Usage: grafana-app-sdk project component add [options] <components>
	where <components> are one or more of:
		backend
		frontend
		operator
```

Since we're building out everything we can as part of this tutorial, let's go ahead and add all three project components.
```shell
grafana-app-sdk project component add frontend backend operator
```
But this gives us an error:
```shell
$ grafana-app-sdk project component add frontend backend operator
plugin-id is required
```

Oops, looks like we need some extra information for this command. We need a `--plugin-id` flag, because it's going to be generating a grafana plugin, which requires that we have a unique ID. We can also view the full list of all flags we can pass to this command with:
```shell
$ grafana-app-sdk project component add --help
Usage:
  grafana-app-sdk project component add [flags]

Flags:
  -h, --help               help for add
      --plugin-id string   Plugin ID

Global Flags:
  -c, --cuepath string      Path to directory with cue.mod (default "kinds")
      --overwrite           Overwrite existing files instead of prompting
  -p, --path string         Path to project directory
  -s, --selectors strings   selectors
```
We can leave all the global flags empty, like we have for other commands, but it's good to know how we can find information about the CLI commands. 

Let's give our plugin an ID (I'm going to use `issue-tracker-project`, but you can use anything you want as long as it won't conflict with another plugin), and run the command again:
```shell
grafana-app-sdk project component add frontend backend operator --plugin-id="issue-tracker-project"
```
Just like with any other command that writes files, the output is a list of all written files:
```shell
$ grafana-app-sdk project component add frontend backend operator --plugin-id="issue-tracker-project"
 * Writing file plugin/.config/.eslintrc
 * Writing file plugin/.config/.prettierrc.js
 * Writing file plugin/.config/Dockerfile
 * Writing file plugin/.config/README.md
 * Writing file plugin/.config/jest/mocks/react-inlinesvg.tsx
 * Writing file plugin/.config/jest/utils.js
 * Writing file plugin/.config/jest-setup.js
 * Writing file plugin/.config/jest.config.js
 * Writing file plugin/.config/tsconfig.json
 * Writing file plugin/.config/types/custom.d.ts
 * Writing file plugin/.config/webpack/constants.ts
 * Writing file plugin/.config/webpack/utils.ts
 * Writing file plugin/.config/webpack/webpack.config.ts
 * Writing file plugin/.eslintrc
 * Writing file plugin/.nvmrc
 * Writing file plugin/.prettierrc.js
 * Writing file plugin/CHANGELOG.md
 * Writing file plugin/LICENSE
 * Writing file plugin/README.md
 * Writing file plugin/jest-setup.js
 * Writing file plugin/jest.config.js
 * Writing file plugin/src/App.tsx
 * Writing file plugin/src/components/Routes/Routes.tsx
 * Writing file plugin/src/components/Routes/index.tsx
 * Writing file plugin/src/module.ts
 * Writing file plugin/src/pages/index.tsx
 * Writing file plugin/src/pages/main.tsx
 * Writing file plugin/src/types.ts
 * Writing file plugin/src/utils/utils.plugin.ts
 * Writing file plugin/src/utils/utils.routing.ts
 * Writing file plugin/tsconfig.json
 * Writing file plugin/src/plugin.json
 * Writing file plugin/src/constants.ts
 * Writing file plugin/package.json
 * Writing file plugin/pkg/main.go
 * Writing file pkg/plugin/handler_issue.go
 * Writing file pkg/plugin/plugin.go
 * Writing file pkg/plugin/secure/data.go
 * Writing file pkg/plugin/secure/middleware.go
 * Writing file pkg/plugin/secure/retriever.go
 * Writing file plugin/Magefile.go
 * Writing file plugin/src/plugin.json
 * Writing file cmd/operator/config.go
 * Writing file cmd/operator/kubeconfig.go
 * Writing file cmd/operator/main.go
 * Writing file pkg/app/app.go
 * Writing file pkg/watchers/watcher_issue.go
 * Writing file cmd/operator/Dockerfile
```
Wow, that's a lot more files written out than in our Kind codegen. Let's take a look at the tree to get a better picture of everything:
```shell
$ tree -I "generated|definitions|kinds|local" .
.
├── Makefile
├── cmd
│   └── operator
│       ├── Dockerfile
│       ├── config.go
│       ├── kubeconfig.go
│       └── main.go
├── go.mod
├── go.sum
├── pkg
│   ├── app
│   │   └── app.go
│   ├── plugin
│   │   ├── handler_issue.go
│   │   ├── plugin.go
│   │   └── secure
│   │       ├── data.go
│   │       ├── middleware.go
│   │       └── retriever.go
│   └── watchers
│       ├── watcher_foo.go
│       └── watcher_issue.go
└── plugin
    ├── CHANGELOG.md
    ├── LICENSE
    ├── Magefile.go
    ├── README.md
    ├── jest-setup.js
    ├── jest.config.js
    ├── package.json
    ├── pkg
    │   └── main.go
    ├── src
    │   ├── App.tsx
    │   ├── components
    │   │   └── Routes
    │   │       ├── Routes.tsx
    │   │       └── index.tsx
    │   ├── constants.ts
    │   ├── module.ts
    │   ├── pages
    │   │   ├── index.tsx
    │   │   └── main.tsx
    │   ├── plugin.json
    │   ├── types.ts
    │   └── utils
    │       ├── utils.plugin.ts
    │       └── utils.routing.ts
    └── tsconfig.json

15 directories, 35 files
```

Excluding our previously-generated files, we can see that we have a few new go packages (`pkg/watchers`, `pkg/plugin`, and `pkg/plugin/secure`), some go files and a Dockerfile in `cmd/operator`, and a bunch of new stuff in the `plugin` directory.

If we had split up our `project add` into `project add backend`, we'd only get our generated go files in `pkg/plugin`, `project add frontend` would only give us the non-`plugin/pkg` plugin files, and `project add operator` would give us the `pkg/watchers` and `cmd/operator` files. As we can see, none of these `project add` components have overlapping code, which is deliberate. If you prefer to not use boilerplate for a given component, you can simply not add it and not worry that another component will depend on boilerplate from it.

So, what are these new bits of code doing?

## Go Code from backend component

**Important note**: the back-end part of the plugin is primarily used as a proxy to the app API server, in order to allow the user to use grafana auth to make the request to the grafana resource API, and let the plugin make the request to the API server using credentials for the API server. The final state of app platform will allow for grafana auth to be used with the API server, and direct access to the API server from outside of the back-end, so the eventual goal is to both allow and encourage the front-end to directly interact with the API server and kubernetes-style APIs.

### `pkg/plugin`

The `project add` didn't actually generate too many files for our back-end boilerplate, just a couple of go files in `pkg/plugin` and then some code in `pkg/plugin/secure`:
```shell
$ tree pkg/plugin
pkg/plugin
├── handler_issue.go
├── plugin.go
└── secure
    ├── data.go
    ├── middleware.go
    └── retriever.go

1 directory, 5 files
```

### Secure JSON Data

The code in the `pkg/plugin/secure` package is focused around defining the shape of your `SecureJSONData`, which is encrypted data shared between the front-end and back-end of the plugin. For more information on data jsonData/secureJSONData, see [this section of grafana's plugin docs](https://grafana.com/docs/grafana/latest/developers/plugins/add-authentication-for-data-source-plugins/#encrypt-data-source-configuration) (it refers to data source plugins, but the concept is the same for all plugins that have a back-end component).

For our purposes, we care about the secureJSONData because we're going to store the details on how to access our storage medium in there: since we're going to be using kubernetes to store our data, we'll have a kubeconfig embedded in the secure JSON data. In your own development, you may store things such as user keys for a third-party service in this data if the back-end needs to reach out to them.

### Plugin Router and Handlers

The code in `pkg/plugin` is split into two files: 
* `plugin.go`, which defines our `Plugin` type we'll run everything from, which embeds a router and defines routes.
* `handler_issue.go`, which defines the handlers for the `issue` routes defined in `plugin.go`. If we had more Kinds, we'd have a handler go file for each one, with boilerplate CRUDL code for each Kind.

The first thing defined in `plugin.go` is a `Service` interface:
```go
type Service interface { 
    GetIssueService(context.Context) (IssueService, error)
}
```
Getting ahead of ourselves, we have a `Service` which returns the actual services our plugin will use (such as `IssueService`), because we have to lazy-instantiate our Schema-specific services. This is because we need data from that `secureJSONData` mentioned above, and we only get that data from a request made to the back-end of the plugin through grafana, so we don't have it at start-up time. We'll take a look at the implementation of `Service` with that lazy-initialization later.

Our boilerplate `Plugin` creates a router and registers routes when created with `New`:
```go
func New(namespace string, service Service) (*Plugin, error) {
	p := &Plugin{
	    router: router.NewJSONRouter(log.DefaultLogger),
			namespace: namespace,
	    service: service,
	}

	p.router.Use(
		kubeconfig.LoadingMiddleware(),
		router.MiddlewareFunc(secure.Middleware))

	// V1 Routes
	v1Subrouter := p.router.Subroute("v1/")
	
	// Issue subrouter
	issueSubrouter := v1Subrouter.Subroute("issues/")
	v1Subrouter.Handle("issues", p.handleIssueList, http.MethodGet)
	issueSubrouter.Handle("{name}", p.handleIssueGet, http.MethodGet)
	issueSubrouter.HandleWithCode("", p.handleIssueCreate, http.StatusCreated, http.MethodPost)
	issueSubrouter.Handle("{name}", p.handleIssueUpdate, http.MethodPut)
	issueSubrouter.HandleWithCode("{name}", p.handleIssueDelete, http.StatusNoContent, http.MethodDelete)
	

	return p, nil
}
```
We can see that this router isn't a standard go http router. Requests that come to the back-end of our plugin are sent through grafana's Resource API, which then passes along a subset of that data to the plugin with gRPC. The `router.JSONRouter` abstracts away that implementation detail (and there are other router flavors in the `router` package), and gives us a router where we can define normal HTTP routes, with handlers that will consume a `router.JSONRequest` (which pulls together all the data we get from the forwarded grafana request), and return either some object which can (and will) be marshaled into JSON, or an error (which will be marshaled into an error response).

There are also two pieces of middleware in use:
```go
p.router.Use(
	kubeconfig.LoadingMiddleware(),
	router.MiddlewareFunc(secure.Middleware))
```
`kubeconfig.LoadingMiddleware()` is middleware managed by the grafana-app-sdk which will pull kube config details from the secureJSONData and place it into the context. We'll see the other side, where we use that kube config, later on.
`router.MiddlewareFunc(secure.Middleware)` is that secureJSONData middleware we just talked about in our boilerplate `pkg/plugin/secure` package.

The last bits in the boilerplate code here are just creating a subrouter for our `issue` Kind and adding routes and handlers for all standard Create/Read/Update/Delete/List endpoints.

The handler functions themselves are defined in `pkg/plugin/handler_issue.go`, though we can see that the first thing defined is our `IssueService`:
```go
type IssueService interface {
	List(ctx context.Context, namespace string, filters ...string) (*resource.TypedStoreList[*issue.Object], error)
	Get(ctx context.Context, id resource.Identifier) (*issue.Object, error)
	Add(ctx context.Context, obj *issue.Object) (*issue.Object, error)
	Update(ctx context.Context, id resource.Identifier, obj *issue.Object) (*issue.Object, error)
	Delete(ctx context.Context, id resource.Identifier) error
}
```
This service is what we'll have to actually implement later when we start writing code, but it's what the handlers are going to try to use to do what they're supposed to do. To see this, let's take a look at the list handler (defined first):
```go
func (p *Plugin) handleIssueList(ctx context.Context, req router.JSONRequest) (router.JSONResponse, error) {
	filtersRaw := req.URL.Query().Get("filters")
	filters := make([]string, 0)
	if len(filtersRaw) > 0 {
		filters = strings.Split(filtersRaw, ",")
	}
	svc, err := p.service.GetIssueService(ctx)
	if err != nil {
	    log.DefaultLogger.Error("Error getting IssueService: " + err.Error())
	    return nil, plugin.NewError(http.StatusInternalServerError, err.Error())
	}
	return svc.List(ctx, p.namespace, filters...)
}
```
It satisfies the `router.JSONHandlerFunc` function type, so that we can use it as a handler. The first parameter, `ctx`, is somewhat self-explanatory as the go context (if you're unfamiliar with go contexts, [the godoc](https://pkg.go.dev/context) is a good place to start). The second parameter is a `router.JSONRequest`. This is a sort of plugin equivalent of the `http.Request`, though with some differences, most of which we won't cover here. The important one to know is that it doesn't have all the request data you might have in an `http.Request`, such as the hostname, or all the headers. The `url.URL` we get with `req.URL` contains a URL which begins at the entrypoint to our API, so the first part will be the first part of the path in our route (no protocol, host, or initial grafana resource API path).

We return a `router.JSONResponse`, which is any JSON-marshalable object, and a possible `error`. The `router.JSONRouter` will handle response marshaling and writing for us, so rather than needing to write out data like in a `http` handler, we just return as we would a normal function.

In our list handler boilerplate, we can see we grab filters from the query, if present, and then we call `List` on our `IssueService` we attempt to retrive from our `Service` implementation. Overall, the handler functions in this file should be pretty straightforward, and you're encouraged to change them as you see fit once we have a working application (this code isn't something that you'll be re-generating, like the `pkg/generated` code).

### `plugin/pkg`

`plugin/pkg` is where the `main` package lives for our plugin, it's what will be compiled for the back-end. This is also where the boilerplate has the most gaps that need to be filled to make things functional, but let's take a look at what's given to us first.

Let's ignore `PluginService` for now, as we'll be replacing that code later with our own, and just take a look at what `main()` does:
```go
func main() {
    svc := &PluginService{}

    // GENERATED SIMPLE SERVICE INITIALIZER CODE
    svc.issueServiceInitializer = kubeconfig.CachingInitializer(
        func(cfg kubeconfig.NamespacedConfig) (plugin.IssueService, error) {
            // This is example code which assumes the API and storage models are identical
            // TODO: REPLACEME
            return resource.NewTypedStore[*issue.Object](issue.Schema(), k8s.NewClientRegistry(cfg.RestConfig))
        })
    

    p, err := plugin.New("default", svc) // TODO: fix namespace usage
    if err != nil {
        panic(err)
    }

    // Start listening
    err = p.Start()
    if err != nil {
        panic(err)
    }
}
```
The important thing to look at is the `kubeconfig.CachingInitializer` being used for the service initializer func. This is another SDK library which allows us to define an initializer for a service which will be called only once per unique kube config. We'll get more in-depth on what this is and why we need to do this when we begin writing our back-end code, but I want to point this out.

Otherwise, the `main()` code is pretty simple. We create a new `plugin.Plugin` with `plugin.New`, and then start it. That's really all there is to it for our `main` package, all the meat of the back-end is going to be in `pkg`, rather than in `plugin`, this is just the "hook" as it were, into all that code.

## Front-End Code from frontend component

A _lot_ of files were generated in `plugin`:
```bash
$ tree plugin
plugin
├── CHANGELOG.md
├── LICENSE
├── Magefile.go
├── README.md
├── jest-setup.js
├── jest.config.js
├── package.json
├── pkg
│   └── main.go
├── src
│   ├── App.tsx
│   ├── components
│   │   └── Routes
│   │       ├── Routes.tsx
│   │       └── index.tsx
│   ├── constants.ts
│   ├── generated
│   │   └── issue
│   │       └── v1
│   │           └── issue_object_gen.ts
│   │           └── types.metadata.gen.ts
│   │           └── types.spec.gen.ts
│   │           └── types.status.gen.ts
│   ├── module.ts
│   ├── pages
│   │   ├── index.tsx
│   │   └── main.tsx
│   ├── plugin.json
│   ├── types.ts
│   └── utils
│       ├── utils.plugin.ts
│       └── utils.routing.ts
└── tsconfig.json

10 directories, 21 files
```
We can also safely _ignore_ a lot of this generation. If you create a grafana plugin, there's a certain amount of metadata that needs to be created, and, likewise, when you create a react app (which front-end plugins are), there's some other data that needs to exist. So basically everything in the root `plugin` directory is something we can ignore for the moment, as it's just things telling either grafana how to handle this app, or react how to compile it. But, as a quick breakdown, here's what each file does that we're going to ignore:

| File | Purpose |
| --- | --- |
|`jest.config.js`|[Jest](https://jestjs.io/) test configuration|
|`Magefile.go`|[Mage](https://magefile.org/) build configuration|
|`package.json`|React app configuration|
|`README.md`|Plugin README (required by grafana)|
|`tsconfig.json`|TypeScript config|
|`src/plugin.json`|Grafana plugin information|

That leaves us with just our varying TypeScript files.

### Pages

`pages/` contains the acual front-end pages to be displayed for the app. `main.tsx` is your main plugin page, which by default just contains a simple statement declaring it your main landing page:

```TypeScript
export const MainPage = () => {
  useStyles2(getStyles);

  return (
      <div>
        <h1>Main Landing Page</h1>
        <div>This is your main landing page</div>
      </div>
  );
};
```

`MainPage` is used by the router when displaying pages--you can add more by creating other exported functions and registering them in the router.

### Router

`components/Routes/Router.tsx` contains the router for your app frontend. By default only the `MainPage` is routed, and matches any path:
```TypeScript
export const Routes = () => {
  useNavigation();

  return (
    <Switch>
      <Route exact path={prefixRoute(ROUTES.Main)} component={MainPage} />

      {/* Default page */}
      <Route exact path="*">
        <Redirect to={prefixRoute(ROUTES.Main)} />
      </Route>
    </Switch>
  );
};
```

`ROUTES.Main` is a constant pulled from `constants.ts`. `useNavigation` and `prefixRoute` are pulled from `utils`.

### Types

`generated/issue/v1` contains the types for our v1 `Issue` kind, which we can use to interact with the plugin backend (and API server).

## Go Code & Dockerfile from operator component

### `pkg/app`

This is the code for the app itself. The app (business logic) and the way it is run (an operator) are treated as separate concepts by the grafana-app-sdk to allow you to run the same app multiple ways based on your needs.
`pkg/app/app.go` contains two exported methods: `Provider` and `New`. `New` creates a new grafana-app-sdk `app.App`-implementing instance (in our case, we use `simple.App` for this), 
and `Provider` returns a new `app.Provider` which packs in your manifest, app-specific config, and the ability to call `New`. 
`app.Provider` is what is used by runners such as the operator runner we created with `component add operator`.

### `cmd/operator`

Here is where the `main` code to run the operator lives, and the docker file to package it as an image for deployment. 
`cmd/operator/main.go` has a straightforward `main` function that:
1. Loads the kube config, assuming that it's running in the cluster that it will work with.
2. Creates the operator runner
3. Runs the operator runner using the `Provider` we generated in the `app` package, stopping on SIGTERM or SIGINT

### `pkg/watchers`

Here we only have one file, created for our Issue kind. If we had more kinds, we'd have more files here, as the `project add operator` command generates a boilerplate watcher for each kind in CUE with a `target: "resource"`. This file defines a simple watcher object which implements `operator.ResourceWatcher`, with an additional `Sync` function which is used in conjunction with a `resource.OpinionatedWatcher`. All this boilerplate watcher does is check that it can cast the provided resource(s) into the `issue.Object` type, and then print a line to the console with the event type and details.

Next, now that we have minimal functioning code, we can try, [deploying our project locally](05-local-deployment.md)

### Prev: [Generating Kind Code](03-generate-kind-code.md)
### Next: [Local Deployment](05-local-deployment.md)