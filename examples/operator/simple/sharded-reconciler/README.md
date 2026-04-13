# Hash Ring Sharding Example

This example shows how to use `simple.BasicReconcileOptions.ShardFilter` with a `dskit` memberlist-backed hash ring.

Two or more replicas can watch the same Kubernetes resources, but only the replica assigned to a given `namespace/name` hash slot will run the reconciler for that object.

Files in this example:

- `main.go`: starts the operator and wires the shard filter into `simple.NewApp`
- `filter.go`: implements the `dskit` ring bootstrap and `simple.ShardFilter`
- `filter_test.go`: covers the shard-assignment logic
- `example.yaml`: sample resources to trigger reconciliation

## Quick Start

### 1. Create a disposable k3d cluster

```sh
k3d cluster create app-sdk-sharding-test --wait
k3d kubeconfig get app-sdk-sharding-test > /tmp/app-sdk-sharding-test.kubeconfig
kubectl --kubeconfig=/tmp/app-sdk-sharding-test.kubeconfig cluster-info
```

### 2. Start replica A

From this directory:

```sh
cd /Users/todd/go/src/github.com/grafana/grafana-app-sdk/examples/operator/simple/sharded-reconciler
```

Run:

```sh
go run . \
  --kubecfg=/tmp/app-sdk-sharding-test.kubeconfig \
  --instance-id=replica-a \
  --memberlist-bind-addr=127.0.0.1 \
  --memberlist-advertise-addr=127.0.0.1 \
  --memberlist-bind-port=7946 \
  --memberlist-advertise-port=7946 \
  --metrics-port=9090
```

### 3. Start replica B

In a second terminal, from the same directory, run:

```sh
go run . \
  --kubecfg=/tmp/app-sdk-sharding-test.kubeconfig \
  --instance-id=replica-b \
  --memberlist-bind-addr=127.0.0.1 \
  --memberlist-advertise-addr=127.0.0.1 \
  --memberlist-bind-port=7947 \
  --memberlist-advertise-port=7947 \
  --memberlist-join=127.0.0.1:7946 \
  --metrics-port=9091
```

Important:

- Set an explicit `--memberlist-advertise-port` for every replica.
- Replica B joins replica A with `--memberlist-join=127.0.0.1:7946`.

### 4. Create test resources

```sh
kubectl --kubeconfig=/tmp/app-sdk-sharding-test.kubeconfig apply -f example.yaml
kubectl --kubeconfig=/tmp/app-sdk-sharding-test.kubeconfig get basiccustomresources.example.grafana.app -A
```

### 5. Watch the logs

Each object should be reconciled by exactly one replica.

Example log line:

```txt
instance=replica-b action=CREATE object=default/sharded-example-a
```

If both replicas are healthy, one replica may handle both sample objects, or they may split between replicas. The important behavior is that each object is processed once by only one replica.

## What The Example Does

`filter.go` starts three `dskit` components:

- a memberlist KV service
- a ring reader
- a ring lifecycler

The filter is added to the app in two ways:

- as `ReconcileOptions.ShardFilter`, so the reconciler only runs on the assigned replica
- as an app runnable, so the ring manager starts and stops with the app

The assignment hash uses:

```txt
kind + "/" + namespace + "/" + name
```

That keeps different kinds from colliding while staying simple for a single-version example.

## Cleanup

Stop the running `go run` processes, then delete the cluster:

```sh
k3d cluster delete app-sdk-sharding-test
```

## Notes

- The example uses `UsePlain: true` so the demo stays focused on sharding, not opinionated finalizer behavior.
- The ring bootstrap in `filter.go` uses `go-kit/log` because that is the logger type required by the `dskit` APIs it integrates with.
- The CRD is registered by the example process on startup.
- You can adapt the memberlist flags for pods or multiple machines by changing the advertise address and join members.
