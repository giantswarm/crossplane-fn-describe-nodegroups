# function-describe-nodegroups

A [Crossplane] Composition Function which reads EKS nodegroup information and
uses that to create `AWSManagedMachinePool` objects

## How it works

> **Warning**
> This plugin is requires Crossplane v1.14 which is currently unreleased
> (due 1st November 2023).
>
> The example composition is also written for Crossplane v1.14 and will
> not work on any current MC version.
>
> To support this, the script [`kind.sh`](./kind.sh) is provided to
> help you understand how this works by spinning crossplane up inside a
> kind cluster for local development.

In order to use this function as part of the [Composition], the composition
must be written to use pipeline mode. This is a (currently undocumented)
mode for compositions.

```yaml
spec:
  compositeTypeRef:
    apiVersion: crossplane.giantswarm.io/v1alpha1
    kind: CompositeEksImport
  mode: Pipeline
  pipeline:
  - step: collect-cluster
    ...
  - step: generate-subnets
    ...
```

