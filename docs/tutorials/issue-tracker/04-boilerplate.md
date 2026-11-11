# Generating Boilerplate

Since this is a fresh project, we can take advantage of the CLI's tooling to set up boilerplate code for us which we can then extend on. Note that this is not strictly necessary for writing an application (whereas running the CUE codegen is something you'll likely want for every project), but it makes initial project bootstrapping simpler, and will help us move along here faster. If you decide in future projects you want to handle your routing, storage, or front-end framework differently, you can eschew some or all of the things laid out in this section.

## The `project component add` Command

Earlier, we used the CLI's `project` command with `project init`, initializing our project with some very basic stuff. Now, we can again use the `project` command, this time to add boilerplate components to our app. These are added using the `project component add` command, with the name of one or more components you wish to add to the project. To see the list of possible components, you can run it sans arguments, like so:
```shell
$ grafana-app-sdk project component add
Usage: grafana-app-sdk project component add [options] <components>
	where <components> are one or more of:
		frontend
		operator
```

We can also get information on what flags this (or any other) command takes with `--help`:
```shell
$ grafana-app-sdk project component add --help
Usage:
  grafana-app-sdk project component add [flags]

Flags:
  -h, --help              help for add

Global Flags:
  -c, --config string   Path selector of the config object within the manifest (default "config")
  -f, --format string   Format in which kinds are written for this project (currently allowed values are 'cue') (default "cue")
      --overwrite       Overwrite existing files instead of prompting
  -p, --path string     Path to project directory
  -s, --source string   Path to directory with your codegen source files (such as a CUE module) (default "kinds")
```

We can leave all these flags empty, as we're fine with the defaults, but it's good to know how we can find information about the CLI commands.

Since we're building out everything we can as part of this tutorial, let's go ahead and add both project components.
```shell
grafana-app-sdk project component add frontend operator
```
Just like with any other command that writes files, the output is a list of all written files, though the front-end files are created with `yarn` and are not listed.
```shell
$ grafana-app-sdk project component add frontend operator
Creating plugin frontend using `yarn create @grafana/plugin` (this may take a moment)...
 * Writing file plugin/src/plugin.json
 * Writing file plugin/src/constants.ts
 * Writing file cmd/operator/config.go
 * Writing file cmd/operator/kubeconfig.go
 * Writing file cmd/operator/main.go
 * Writing file pkg/app/app.go
 * Writing file pkg/watchers/watcher_issue.go
 * Writing file cmd/operator/Dockerfile
```
Let's take a look at the tree to get a better picture of everything:
```shell
$ tree -I "generated|definitions|kinds|local" .
.
├── cmd
│   └── operator
│       ├── config.go
│       ├── Dockerfile
│       ├── kubeconfig.go
│       └── main.go
├── go.mod
├── go.sum
├── Makefile
├── pkg
│   ├── app
│   │   └── app.go
│   └── watchers
│       └── watcher_issue.go
└── plugin
    ├── CHANGELOG.md
    ├── docker-compose.yaml
    ├── jest-setup.js
    ├── jest.config.js
    ├── LICENSE
    ├── package.json
    ├── playwright.config.ts
    ├── provisioning
    │   └── plugins
    │       ├── apps.yaml
    │       └── README.md
    ├── README.md
    ├── src
    │   ├── components
    │   │   ├── App
    │   │   │   ├── App.test.tsx
    │   │   │   └── App.tsx
    │   │   ├── AppConfig
    │   │   │   ├── AppConfig.test.tsx
    │   │   │   └── AppConfig.tsx
    │   │   └── testIds.ts
    │   ├── constants.ts
    │   ├── img
    │   │   └── logo.svg
    │   ├── module.tsx
    │   ├── pages
    │   │   ├── PageFour.tsx
    │   │   ├── PageOne.tsx
    │   │   ├── PageThree.tsx
    │   │   └── PageTwo.tsx
    │   ├── plugin.json
    │   ├── README.md
    │   └── utils
    │       └── utils.routing.ts
    ├── tests
    │   ├── appConfig.spec.ts
    │   ├── appNavigation.spec.ts
    │   └── fixtures.ts
    └── tsconfig.json

17 directories, 38 files
```

Excluding our previously-generated files, we can see that we have a few new go packages (`pkg/app` and `pkg/watchers`), some go files and a Dockerfile in `cmd/operator`, and a bunch of new stuff in the `plugin` directory.

If we had split up our `project component add` into `project component add frontend`, we'd only get the files in `plugin`, and `project component add operator` would give us the `pkg/app`, `pkg/watchers` and `cmd/operator` files. As we can see, these components don't have overlapping code, which is deliberate. If you prefer to not use boilerplate for a given component, you can simply not add it and not worry that another component will depend on boilerplate from it.

> [!NOTE]
> Earlier versions of the SDK also had a `backend` component, which generated a plugin backend in `pkg/plugin` that proxied requests to the API server.
> It was removed in v0.61 (see the [v0.61 migration guide](../../migrations/v0.61.md)). Our front-end will talk directly to the API server at
> `/apis/issuetrackerproject.ext.grafana.app/v1alpha1` instead.

So, what are these new bits of code doing?

## Front-End Code from frontend component

A _lot_ of files were generated in `plugin`:
```bash
$ tree plugin
plugin
├── CHANGELOG.md
├── docker-compose.yaml
├── jest-setup.js
├── jest.config.js
├── LICENSE
├── package.json
├── playwright.config.ts
├── provisioning
│   └── plugins
│       ├── apps.yaml
│       └── README.md
├── README.md
├── src
│   ├── components
│   │   ├── App
│   │   │   ├── App.test.tsx
│   │   │   └── App.tsx
│   │   ├── AppConfig
│   │   │   ├── AppConfig.test.tsx
│   │   │   └── AppConfig.tsx
│   │   └── testIds.ts
│   ├── constants.ts
│   ├── generated
│   │   └── issue
│   │       └── v1alpha1
│   │           ├── issue_object_gen.ts
│   │           ├── types.metadata.gen.ts
│   │           ├── types.spec.gen.ts
│   │           └── types.status.gen.ts
│   ├── img
│   │   └── logo.svg
│   ├── module.tsx
│   ├── pages
│   │   ├── PageFour.tsx
│   │   ├── PageOne.tsx
│   │   ├── PageThree.tsx
│   │   └── PageTwo.tsx
│   ├── plugin.json
│   ├── README.md
│   └── utils
│       └── utils.routing.ts
├── tests
│   ├── appConfig.spec.ts
│   ├── appNavigation.spec.ts
│   └── fixtures.ts
└── tsconfig.json

14 directories, 33 files
```
We can also safely _ignore_ a lot of this generation. If you create a grafana plugin, there's a certain amount of metadata that needs to be created, and, likewise, when you create a react app (which front-end plugins are), there's some other data that needs to exist. So basically everything in the root `plugin` directory is something we can ignore for the moment, as it's just things telling either grafana how to handle this app, or react how to compile it. But, as a quick breakdown, here's what each file does that we're going to ignore:

| File | Purpose |
| --- | --- |
|`jest.config.js`|[Jest](https://jestjs.io/) test configuration|
|`package.json`|React app configuration|
|`README.md`|Plugin README (required by grafana)|
|`tsconfig.json`|TypeScript config|
|`src/plugin.json`|Grafana plugin information|

That leaves us with just our varying TypeScript files.

### Pages

`pages/` contains the actual front-end pages to be displayed for the app. `PageOne.tsx` is your main plugin page, which by default just contains a simple statement declaring it your main landing page:

```TypeScript
function PageOne() {
  const s = useStyles2(getStyles);

  return (
    <PluginPage>
      <div data-testid={testIds.pageOne.container}>
        This is page one.
        <div className={s.marginTop}>
          <LinkButton data-testid={testIds.pageOne.navigateToFour} href={prefixRoute(ROUTES.Four)}>
            Full-width page example
          </LinkButton>
        </div>
      </div>
    </PluginPage>
  );
}
```
There are other pages generated here as examples from the output of `yarn create @grafana/plugin`. The routing for these pages is in `components/App/App.tsx`.

`MainPage` is used by the router when displaying pages--you can add more by creating other exported functions and registering them in the router.

### Components

`components` contains your front-end App declaration and AppConfig.

### Types

`generated/issue/v1` contains the types for our v1 `Issue` kind, which we can use to interact with the API server.

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

Here we only have one file, created for our Issue kind. If we had more kinds, we'd have more files here, as the `project component add operator` command generates a boilerplate watcher for each kind in CUE with a `target: "resource"`. This file defines a simple watcher object which implements `operator.ResourceWatcher`, with an additional `Sync` function which is used in conjunction with a `resource.OpinionatedWatcher`. All this boilerplate watcher does is check that it can cast the provided resource(s) into the `issue.Object` type, and then print a line to the console with the event type and details.

Next, let's write the [front-end](05-frontend.md).

### Prev: [Generating Kind Code](03-generate-kind-code.md)
### Next: [Writing Our Front-End](05-frontend.md)
