# Syncing Issues to the Knowledge Graph

This chapter is optional. It needs a Grafana Cloud stack with Knowledge Graph enabled, and skipping it doesn't affect the rest of the tutorial.

Our watcher already reacts to every Issue change, no matter who made it. That makes it a good place to copy issues into [Grafana Cloud Knowledge Graph](https://grafana.com/docs/grafana-cloud/knowledge-graph/), where each Issue becomes an entity you can query and relate to other entities. We'll extend `IssueWatcher` to do three things:
* Upsert an `Issue` entity on add, update, and sync
* Delete the entity when the Issue is deleted
* Create a `BLOCKS` relationship between two Issues, driven by a label

## The Knowledge Graph write client

Grafana publishes a generated Go client for the Knowledge Graph write API in [grafana/grafana-asserts-public-clients](https://github.com/grafana/grafana-asserts-public-clients). It has no tagged releases, so `go get` it at a commit (Go turns that into a pseudo-version):
```shell
go get github.com/grafana/grafana-asserts-public-clients/go/kgwrite@<commit-sha>
```
The client authenticates with HTTP Basic auth, using your stack ID as the username and a Grafana Cloud access token as the password. It talks to your stack's cell gateway URL, which looks like `https://asserts-dev-us-central-0.grafana-dev.net` (yours will differ). We'll read all three values from the environment:
```shell
export KG_GATEWAY_URL=<your gateway URL>
export GRAFANA_STACK_ID=<your stack ID>
export GCOM_TOKEN=<your token>
```
Entities live in the namespace `stacks-<stackId>`, and every call also sends your stack ID in the `X-Scope-OrgID` header.

Let's put the client in its own file, `pkg/watchers/knowledgegraph.go`:
```go
package watchers

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/grafana/grafana-asserts-public-clients/go/kgwrite"
)

const (
	kgDomain     = "issuetracker"
	kgEntityType = "Issue"
	kgNoExpiry   = int64(-1)
)

type KnowledgeGraph struct {
	client    *kgwrite.APIClient
	stackID   string
	namespace string
}

func NewKnowledgeGraphFromEnv() (*KnowledgeGraph, error) {
	baseURL, stackID, token := os.Getenv("KG_GATEWAY_URL"), os.Getenv("GRAFANA_STACK_ID"), os.Getenv("GCOM_TOKEN")
	if baseURL == "" || stackID == "" || token == "" {
		return nil, fmt.Errorf("KG_GATEWAY_URL, GRAFANA_STACK_ID, and GCOM_TOKEN must be set")
	}
	cfg := kgwrite.NewConfiguration()
	cfg.Servers = kgwrite.ServerConfigurations{{URL: baseURL}}
	cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	cfg.AddDefaultHeader("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(stackID+":"+token)))
	return &KnowledgeGraph{
		client:    kgwrite.NewAPIClient(cfg),
		stackID:   stackID,
		namespace: "stacks-" + stackID,
	}, nil
}
```
The domain must match `^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$` and can't be `kg` or `metrics`, which is why we use `issuetracker`. The entity type must match `^[A-Za-z][A-Za-z0-9_]*$`. A TTL of `-1` means the entity never expires. That's safe here because we delete entities ourselves, as we'll see below.

## Mapping an Issue to an entity

An entity is identified by its domain, type, name, and scope. We'll use the Issue's `metadata.name` as the name, and its `metadata.namespace` as the scope. Our local namespace is `default`, while the Knowledge Graph tenant comes from the stack ID, so the two don't collide. `title` and `status` become properties:
```go
func issueScope(issue *issuev1alpha1.Issue) map[string]string {
	return map[string]string{"namespace": issue.GetNamespace()}
}

func (k *KnowledgeGraph) UpsertIssue(ctx context.Context, issue *issuev1alpha1.Issue) error {
	title, status := issue.Spec.Title, issue.Spec.Status
	entity := kgwrite.NewEntityWriteRequestDto(kgDomain, kgEntityType, issue.GetName(), kgNoExpiry)
	entity.SetScope(issueScope(issue))
	entity.SetProperties(map[string]kgwrite.KgPropertyValue{
		"title":  kgwrite.StringAsKgPropertyValue(&title),
		"status": kgwrite.StringAsKgPropertyValue(&status),
	})
	_, _, err := k.client.KnowledgeGraphWriteAPIAPI.UpsertEntity(ctx, k.namespace).
		XScopeOrgID(k.stackID).
		AllowUndeclared(true).
		EntityWriteRequestDto(*entity).
		Execute()
	if err != nil {
		return fmt.Errorf("upserting issue %s: %w", issue.GetName(), err)
	}
	return nil
}
```
Property values must be scalars or arrays of scalars. `AllowUndeclared(true)` sets the `allowUndeclared` query parameter. We need it because `Issue` isn't declared in any installed schema. The server answers 201 when it creates the entity and 200 when it updates one, so a repeated upsert is harmless.

Deleting is the mirror image, and answers 204:
```go
func (k *KnowledgeGraph) DeleteIssue(ctx context.Context, issue *issuev1alpha1.Issue) error {
	_, err := k.client.KnowledgeGraphWriteAPIAPI.DeleteEntity(ctx, k.namespace).
		XScopeOrgID(k.stackID).
		Domain(kgDomain).
		Type_(kgEntityType).
		Name(issue.GetName()).
		Scope(issueScope(issue)).
		Execute()
	if err != nil {
		return fmt.Errorf("deleting issue %s: %w", issue.GetName(), err)
	}
	return nil
}
```
Only entities written through this API can be written or deleted through it.

## Relating issues

The `Issue` schema has no reference fields, so we'll express "this issue blocks that one" with a label, `issuetracker.ext.grafana.app/blocks`, whose value is the name of another Issue in the same namespace:
```go
const blocksLabel = "issuetracker.ext.grafana.app/blocks"

func (k *KnowledgeGraph) UpsertBlocks(ctx context.Context, issue *issuev1alpha1.Issue) error {
	target, ok := issue.GetLabels()[blocksLabel]
	if !ok {
		return nil
	}
	from := *kgwrite.NewEntityRefDto(kgDomain, kgEntityType, issue.GetName())
	from.SetScope(issueScope(issue))
	to := *kgwrite.NewEntityRefDto(kgDomain, kgEntityType, target)
	to.SetScope(issueScope(issue))
	_, _, err := k.client.KnowledgeGraphWriteAPIAPI.UpsertRelationship(ctx, k.namespace).
		XScopeOrgID(k.stackID).
		AllowUndeclared(true).
		RelationshipWriteRequestDto(*kgwrite.NewRelationshipWriteRequestDto(kgDomain, "BLOCKS", from, to, kgNoExpiry)).
		Execute()
	if err != nil {
		return fmt.Errorf("upserting BLOCKS %s -> %s: %w", issue.GetName(), target, err)
	}
	return nil
}
```
Both endpoints must already exist, and the API doesn't create stub entities. If the target Issue hasn't synced yet, the call fails with a 422. We return that error and let the retry policy try again later, by which point the target will usually be there. `DeleteRelationship` removes a relationship. We skip it here, so removing or changing the label leaves the old `BLOCKS` edge in place until one of its Issues is deleted.

## Calling it from the watcher

The watcher now needs the Knowledge Graph client, so we add it to the struct and constructor (existing code omitted):
```go
type IssueWatcher struct {
	// ...existing fields omitted
	kg *KnowledgeGraph
}

func NewIssueWatcher(clientGenerator resource.ClientGenerator, kg *KnowledgeGraph) (*IssueWatcher, error) {
	// ...existing code omitted
	return &IssueWatcher{
		// ...existing fields omitted
		kg: kg,
	}, nil
}
```
Add, Update, and Sync all do the same thing, so they share a helper:
```go
func (s *IssueWatcher) syncKnowledgeGraph(ctx context.Context, issue *issuev1alpha1.Issue) error {
	if err := s.kg.UpsertIssue(ctx, issue); err != nil {
		return err
	}
	return s.kg.UpsertBlocks(ctx, issue)
}
```
In `Add` and `Sync`, call it right after the status update and before the gauge code, for the same reason as before: an error means a retry, and we don't want to bump the gauge twice.
```go
if err := s.syncKnowledgeGraph(ctx, object); err != nil {
	return err
}
```
In `Update`, do the same with `newObject`. In `Delete`, call `s.kg.DeleteIssue(ctx, object)` the same way.

`simple.App` wraps our watcher in an `OpinionatedWatcher` by default. It adds the finalizer you saw in the `kubectl` output in chapter 6, so a delete that happens while the operator is down still reaches `Delete` once it comes back. It also calls `Sync` on startup for every existing Issue. Between the two, the graph converges even after an outage, which is why entities don't need a TTL.

Finally, update `pkg/app/app.go` to build the client and pass it in:
```go
kg, err := watchers.NewKnowledgeGraphFromEnv()
if err != nil {
	return nil, fmt.Errorf("unable to create KnowledgeGraph client: %w", err)
}
issueWatcher, err := watchers.NewIssueWatcher(k8s.NewClientRegistry(cfg.KubeConfig, k8s.DefaultClientConfig()), kg)
```
Set the three environment variables on the operator's deployment (for the local setup, in the operator's environment in your Tilt config), then rebuild and redeploy:
```shell
make build/operator && make local/push_operator
```

## Checking the result

Create an issue:
```shell
echo '{"kind":"Issue","apiVersion":"issuetrackerproject.ext.grafana.app/v1alpha1","metadata":{"name":"test-issue","namespace":"default"},"spec":{"title":"Foo","description":"bar","status":"open"}}' | kubectl create -f -
```
Then look it up with the read API. The same `kgwrite` client has it:
```go
entity, _, err := client.KnowledgeGraphReadAPIAPI.LookupEntity(ctx, "stacks-"+stackID).
	XScopeOrgID(stackID).
	Domain("issuetracker").
	Type_("Issue").
	Name("test-issue").
	Scope(map[string]string{"namespace": "default"}).
	Execute()
```
or, with curl:
```shell
curl -g -u '<stack-id>:<gcom-token>' -H 'X-Scope-OrgID: <stack-id>' \
  "$KG_GATEWAY_URL/apis/kg.grafana.com/v1alpha1/namespaces/stacks-<stack-id>/entities?domain=issuetracker&type=Issue&name=test-issue&scope[namespace]=default"
```
Delete the issue with `kubectl delete issue test-issue` and the same lookup should find nothing.

If a write fails, the operator logs the status code. The ones you're most likely to see:
* `403`: write access is disabled for your tenant.
* `409`: an entity with that identity already exists in a telemetry domain, or wasn't created through this API.
* `422`: validation failed, for example a bad domain or type name, or a relationship endpoint that doesn't exist yet.

The write API also has a batch endpoint, `WriteGraph`, which takes entities and relationships together (up to 500 items) and applies them atomically and idempotently. It's a better fit than one call per object if you ever need to backfill a large number of issues.

This integration is hand-written. A future SDK release may let you opt in from the app manifest instead.

### Prev: [Adding Admission Control](07-adding-admission-control.md)
### Next: [Wrap-Up and Further Reading](09-wrap-up.md)
