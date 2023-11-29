# Front-End

Alright, now that we've verified that we have a working back-end of our plugin, and have a local deployment where we can browse to our UI, it's time to make the front-end work work, and look a bit nicer.

We're still going to keep our front-end pretty simple, so all we're going to do is have our landing page list our issues, allow a user to create a new issue, and be able to update an issue's status, or delete an issue. To allow for this, we'll need a client we can use to do all these things. 

## API Client

In the future, the `project add frontend` will auto-generate this boilerplate client, but for now, we have to write it ourselves. 
For this tutorial, we have one pre-written, that we'll discuss a few parts of. Either create a new file called `plugin/src/api/issue_client.ts` and [copy the contents of this file into it](frontend-files/issue-client.ts), or run the following to do it automatically:
```bash
curl -o plugin/src/api/issue_client.ts https://raw.githubusercontent.com/grafana/grafana-app-sdk/main/docs/tutorials/issue-tracker/frontend-files/issue-client.ts
```

A few things to note in our client:
```TypeScript
import { Issue as generatedIssue } from '../generated/issue_types.gen';

// ommitted code

export interface Issue extends generatedIssue {
    staticMetadata: {
      name: string
      namespace: string
    }
}
```
We import the generated `Issue` interface, but we extend it with `staticMetadata`. Why do we do this? 
Well, the generated interface contains only what we defined in our schema, which doesn't include the metadata contents. 
Since right now we are returning our `resource` Object, and accepting the same type in our POST/PUT requests, we need to include the necessary metadata. 
Later on, we'll separate out our `resource` Object and our API models, but for now, this is necessary to include the `name` and `namespace` identifiers required by the API.

The rest of the client uses grafana libraries to make fetch requests to perform relevent actions. We have methods for `get`, `list`, `create`, `update`, and `delete`. We'll use these methods in our update to the main page of the plugin.

## Main Page

We already have a very empty generated main page located at `plugin/src/pages/main.tsx`. We're going to overwrite all of this with new contents. 
Either copy [the contents of this file](frontend-files/main.tsx) into `plugin/src/pages/main.tsx` (overwriting the current contents), or do it with curl:
```bash
curl -o plugin/src/pages/main.tsx https://raw.githubusercontent.com/grafana/grafana-app-sdk/main/docs/tutorials/issue-tracker/frontend-files/main.tsx
```

TODO: breakdown of the file contents?

## Redeploy

Now we want to redeploy our plugin front-end to see the changes. Since we don't need to rebuild the operator or the plugin's backend, we can just do
```bash
$ make build/plugin-frontend
```
After that completes, we can redploy to our active local environment with
```bash
$ make local/deploy_plugin
```
And just like that, we can refresh or go to [http://grafana.k3d.localhost:9999/a/issue-tracker-project-app], and see our brand-new plugin UI. 
If we create a new issue, we can see that it shows up in the list, or via a `kubectl get issues`.

Now all that's left is to think a bit about our operator.

### Prev: [Local Deployment](05-local-deployment.md)
### Next: [Adding Operator Code](07-operator-watcher.md)