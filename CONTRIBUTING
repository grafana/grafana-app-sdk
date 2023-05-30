# Contributing guide

All contributions are welcome.

If there is something you're curious about with the SDK (project direction, functionality, etc.), please do not hesitate to visit our [Discussions](https://github.com/grafana/grafana-app-sdk/discussions) section.

If you discover a bug, or think something should be included in the SDK, feel free to file an issue, or open a PR.

## Releasing a new version

In order to release a new version, you can use the `scripts/release.sh` script, like so:

```sh
# Release a new patch version, e.g. 1.0.1
./scripts/release.sh patch

# Release a new minor version, e.g. 1.1.0
./scripts/release.sh minor

# Release a new major version, e.g. 2.0.0
./scripts/release.sh major
```

The script will make sure that you have the latest `main` version in your tree, will run linter / tests / build and it will create a semver-appropriate signed tag and push it to remote. Our CI automation will in turn create an appropriate Github release with artifacts. The script currently does not support pre-release versions.
