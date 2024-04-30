# Tutorial: Creating a Simple Issue Tracker in Grafana

This tutorial is meant to guide you from being entirely new to the `grafana-app-sdk` to being able to write your own bespoke applications to run on grafana as a platform. To do so, we'll walk through creating a simple issue tracker app for grafana, and run it locally (or on a kubernetes cluster of your choosing). Along the way, we'll cover all the SDK's CLI tooling, and a lot of the SDK's library packages. In the end, we'll have an app which allows users to create, list, update, and delete "issues," as well as responds to changes in issue status by taking some arbitrary action.

## Pre-requisites

For this tutorial, we make the following assumptions:
* You have at least a basic understanding of [Go](https://go.dev/). This is the language back-end code for grafana apps is written in.
* You have at least a basic understanding of [TypeScript](https://www.typescriptlang.org/). This is the language front-end code for grafana is written in.
Some parts of this tutorial will have us writing in [CUE](https://cuelang.org/), but no familiarity with the language is assumed--we'll break down everything we're writing there.

You'll also need Go installed on your machine to compile the code, as well as the following tools for plugin development:
* [Yarn](https://yarnpkg.com/)
* [Mage](https://magefile.org/)

For running the local environment, you'll need:
* [Docker](https://www.docker.com/get-started/) or [Podman](https://podman.io/getting-started/installation) (this is also required for building the operator)
* [K3D](https://k3d.io)
* [Tilt](https://tilt.dev)

We'll also bring these up again when they become relevant in our process.

OK, with all that out of the way, let's jump in with [Initializing Our Project](01-project-init.md), or you can select a section from the chapter list below.

## Sections

1. [Initializing Our Project](01-project-init.md)
2. [Defining Our Kinds & Schemas](02-defining-our-kinds.md)
3. [Generating Kind Code](03-generate-kind-code.md)
4. [Generating Boilerplate](04-boilerplate.md)
5. [Local Deployment](05-local-deployment.md)
6. [Writing Our Front-End](06-frontend.md)
7. [Adding Operator Code](07-operator-watcher.md)
8. [Adding Admission Control](08-adding-admission-control.md)
9. [Wrap-Up and Further Reading](09-wrap-up.md)