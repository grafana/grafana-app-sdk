# Front-End

Alright, now that we've verified that we have a working back-end of our plugin, and have a local deployment where we can browse to our UI, it's time to make the front-end work, and look a bit nicer.

We're still going to keep our front-end pretty simple, so all we're going to do is have our landing page list our issues, allow a user to create a new issue, and be able to update an issue's status, or delete an issue. To allow for this, we'll need a client we can use to do all these things. 

## API Client

In the future, the `project add frontend` will auto-generate this boilerplate client, but for now, we have to write it ourselves. 
For this tutorial, we have one pre-written, that we'll discuss a few parts of. Either create a new file called `plugin/src/api/issue_client.ts` and [copy the contents of this file into it](frontend-files/issue-client.ts), or run the following to do it automatically:
```bash
mkdir -p plugin/src/api && curl -o plugin/src/api/issue_client.ts https://raw.githubusercontent.com/grafana/grafana-app-sdk/main/docs/tutorials/issue-tracker/frontend-files/issue-client.ts
```

The client uses grafana libraries to make fetch requests to perform relevant actions, and uses the generated `Issue` type in `generated/issue/v1/issue_object_gen.ts` that mirrors our generated go `v1alpha1.Issue` type. We have methods for `get`, `list`, `create`, `update`, and `delete`. We'll use these methods in our update to the main page of the plugin.

## Main Page

We already have a main page located at `plugin/src/pages/PageOne.tsx`. We're going to overwrite all of this with new contents.
Either copy [the contents of this file](frontend-files/main.tsx) into `plugin/src/pages/PageOne.tsx` (overwriting the current contents), or do it with curl:
```bash
curl -o plugin/src/pages/PageOne.tsx https://raw.githubusercontent.com/grafana/grafana-app-sdk/main/docs/tutorials/issue-tracker/frontend-files/main.tsx
```

We've now updated the main page to display a list of our `Issue` resources, with some basic options.

```TypeScript
const [issuesData, setIssuesData] = useState(issues);
useEffect(() => {
    const fetchData = async () => {
        const client = new IssueClient()
        const issues = await client.list();
        setIssuesData(issues.data.items);
    }

    fetchData().catch(console.error);
}, []);
```
Here we list issues using the `IssueClient` we created in the previous section, and push them into the state, so that our issue list returned by this function is populated with the initial list of issues from the API server.

Next we define a few functions to call to manipulate issues:
```TypeScript
// IssueClient to share for all our functions
const ic = new IssueClient();

const listIssues = async() => {
    const issues = await ic.list();
    setIssuesData(issues.data.items);
}

const createIssue = async (title: string, description: string) => {
    await ic.create(title, description);
    await listIssues();
};

const deleteIssue = async (id: string) => {
    await ic.delete(id);
    await listIssues();
};

const updateStatus = async (issue: Issue, newStatus: string) => {
issue.spec.status = newStatus;
    await ic.update(issue.metadata.name, issue);
    await listIssues();
}
```

Finally, we have to display the information. We define one more function:
```tsx
const getActions = (issue: Issue) => {
    if (issue.spec.status === 'open') {
        return (
            <Card.Actions>
                <Button
                    key="mark-in-progress"
                    onClick={() => {
                        updateStatus(issue, 'in_progress');
                    }}
                >
                    Start Progress
                </Button>
            </Card.Actions>
        );
    } else if (issue.spec.status === 'in_progress') {
        return (
            <Card.Actions>
                <Button
                    key="mark-open"
                    onClick={() => {
                        updateStatus(issue, 'open');
                    }}
                >
                    Stop Progress
                </Button>
                <Button
                    key="mark-closed"
                    onClick={() => {
                        updateStatus(issue, 'closed');
                    }}
                >
                    Complete
                </Button>
            </Card.Actions>
        );
    } else {
        return <Card.Actions></Card.Actions>;
    }
};

```
We'll use this information in the display output to show the current issue status and to display the button(s) used to transition its status. After that, it’s simply a matter of returning the appropriate HTML from our component, populating it with data from `issuesData`, which will contain the list of issues retrieved from the API server.

## Redeploy

Now we want to redeploy our plugin front-end to see the changes. Since we don't need to rebuild the operator or the plugin's backend, we can just do
```bash
make build/plugin-frontend
```
After that is completed, we can redeploy to our active local environment with
```bash
make local/deploy_plugin
```
And just like that, we can refresh or go to [http://grafana.k3d.localhost:9999/a/issuetrackerproject-app/], and see our brand-new plugin UI. 
If we create a new issue, we can see that it shows up in the list, or via a `kubectl get issues`.

Now all that's left is to think a bit about our operator.

### Prev: [Local Deployment](05-local-deployment.md)
### Next: [Adding Operator Code](07-operator-watcher.md)