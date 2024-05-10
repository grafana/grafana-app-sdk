# Updating this fork of deepmap/oapi-codegen

The contents of the `oapi-codegen` are copied from the `deepmap/oapi-codegen@v2.1.0`, with the following changes:
* All packages except `pkg/codegen` and `pkg/util` have been removed, as they are not needed for go types generation.
* Package refs have been changed from `github.com/deepmap/oapi-codegen` to `github.com/grafana/thema/internal/deepmap/oapi-codegen`
* The contents of [this PR](https://github.com/deepmap/oapi-codegen/pull/717) in deepmap/oapi-codegen have been played onto this fork

A full diff of changes can be found at [diff.txt].

When updating this fork, please add whatever changes you make (if different from the main oapi-codegen branch) to the above list and update [diff.txt] accordingly. 
This can be done with:
```shell
$ diff <path_to_deepmap>/oapi-codegen/pkg/codegen <path_to_grafana>/grafana-app-sdk/internal/deepmap/oapi-codegen/pkg/codegen > <path_to_grafana>/grafana-app-sdk/internal/deepmap/diff.txt
```
If you make changes to other packages, please also include them in the diff.