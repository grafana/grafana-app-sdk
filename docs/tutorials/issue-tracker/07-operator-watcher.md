# Writing Operator Code

We actually already have some simple operator code generated for us by our `project add` command earlier, so instead of writing new code, let's just talk a bit about what the operator does, and ways of running it.

By default, the operator is run as a separate container alongside your grafana deployment. For simple cases, where there will only be one instance of grafana running, it can be run as an embedded part of your plugin, but that comes with several caveats, namely, that it can't _start_ running until you browse to your plugin page in grafana itself. So, the ordinary use-case for the operator is as a separate deployment.

The operator is a logical pattern which runs one or more controllers. The typical use-case for a controller is the `operator.InformerController`, which holds:
* One or more informers, which subscribe to events for a particular resource kind and namespace
* One or more watchers, which consume events for particular kinds
In our case, we have these lines in our boilerplate `cmd/operator/main.go`:
```golang
issueInformer, err := operator.NewKubernetesBasedInformer(issue.Schema(), issueClient, kubeConfig.Namespace)
if err != nil {
	panic(err)
}
err = controller.AddInformer(issueInformer, issue.Schema().Kind())
if err != nil {
	panic(err)
}
```
Creating an informer which receives events for our Issue kind (and adding it to the controller), and this:
```go
issueWatcher, err := watchers.NewIssueWatcher()
if err != nil {
	panic(err)
}
issueOpinionatedWatcher, err := operator.NewOpinionatedWatcher(issue.Schema(), issueClient)
if err != nil {
	panic(err)
}
issueOpinionatedWatcher.Wrap(issueWatcher, false)
issueOpinionatedWatcher.SyncFunc = issueWatcher.Sync
err = controller.AddWatcher(issueOpinionatedWatcher, issue.Schema().Kind())
if err != nil {
	panic(err)
}
```
Creating a watcher for receiving the events. It wraps this watcher in an `operator.OpinionatedWatcher`, which adds some control logic around using finalizers to make sure delete events can't be missed if the operator is down, and adds that occur during downtime are still processed. It also gives us a `SyncFunc`, which fires on startup for every Issue that was known the the controller _before_ the downtime, but may have updated during it. It's generally good practice to use the `operator.OpinionatedWatcher`, unless you want to implement this kind of logic yourself.

The other thing we're doing here is calling `watchers.NewIssueWatcher()`. The `watchers` package was added by our `project add operator` command, so let's take a look at what's there:
```go
var _ operator.ResourceWatcher = &IssueWatcher{}

type IssueWatcher struct{}

func NewIssueWatcher() (*IssueWatcher, error) {
	return &IssueWatcher{}, nil
}
```
So we have `IssueWatcher`, which implements `operator.ResourceWatcher`. The `Add`, `Update`, `Delete`, and `Sync` functions are all relatively self-explanatory, but let's examine the `Add` one just to be on the same page:
```go
// Add handles add events for issue.Issue resources.
func (s *IssueWatcher) Add(ctx context.Context, rObj resource.Object) error {
	object, ok := rObj.(*issue.Issue)
	if !ok {
		return fmt.Errorf("provided object is not of type *issue.Issue (name=%s, namespace=%s, kind=%s)",
			rObj.StaticMetadata().Name, rObj.StaticMetadata().Namespace, rObj.StaticMetadata().Kind)
	}

	// TODO
	fmt.Println("Added ", object.StaticMetadata().Identifier())
	return nil
}
```
Each method does a check to see if the provided `resource.Object` is of type `*issue.Issue` (it always should be, provided we gave the informer a client with the correct `resource.Schema`). We then just print a line declaring what resource was added, which we saw when [testing our local deployment](05-local-deployment.md).

So what else can we do in our watcher?

Well, right now, we could integrate with some third-party service, maybe you want to sync the issues created in you plugin with GitHub, or some internal issue-tracking tool. You may have some other task which should be performed when an issue is added, or updated, or deleted, which you should do in the operator. As more of grafana begins to use a kubernetes-like storage system, you could even create a resource of another kind in response to an add event, which some other operator would pick up and do something with. Why not do these things in the plugin backend?

Well, as we saw before, your plugin API isn't the only way to interact with Issues. You can create, update, or delete them via `kubectl`. But even if you restrict `kubectl` access, but perhaps another plugin operator may want to create an Issue in response to one of _their_ events. If they did that via directly interfacing with the storage layer, you wouldn't notice that it happened. The operator ensures that no matter _how_ the mutation in the storage layer occurred (API, kubectl, other access), you are informed and can take action.

### Prev: [Writing Our Front-End](06-frontend.md)
### Next: [Wrap-Up and Further Reading](08-wrap-up.md)