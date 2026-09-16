import React from 'react';
import { useForm } from 'react-hook-form';
import {  Button, IconButton, Field, Input, Card, TagList } from '@grafana/ui';
import { Issue } from '../generated/issue/v1alpha1/issue_object_gen';
import {
    useListIssueQuery,
    useCreateIssueMutation,
    useReplaceIssueMutation,
    useDeleteIssueMutation,
} from '../generated/issue/v1alpha1/issue_api_gen';
import { PluginPage } from '@grafana/runtime';

// This is used for the create new issue form
type ReactHookFormProps = {
    title: string;
    description: string;
};

function PageOne() {
    // The list refetches automatically after every mutation below, as they all invalidate the 'Issue' tag.
    const { data } = useListIssueQuery();
    const issuesData = data?.items ?? [];
    const [create] = useCreateIssueMutation();
    const [replace] = useReplaceIssueMutation();
    const [remove] = useDeleteIssueMutation();

    const createIssue = async (title: string, description: string) => {
        await create({
            issue: {
                metadata: { name: 'issue-' + Math.random().toString(36).slice(2, 12) },
                spec: { title, description, status: 'open' },
            } as Issue,
        });
    };

    const deleteIssue = async (name: string) => {
        await remove({ name });
    };

    const updateStatus = async (issue: Issue, newStatus: string) => {
        await replace({ name: issue.metadata.name, issue: { ...issue, spec: { ...issue.spec, status: newStatus } } });
    }

    // Form handling
    const { handleSubmit, register } = useForm<ReactHookFormProps>({
        mode: 'onChange',
        defaultValues: {
            title: '',
            description: '',
        },
    });

    const handleCreate = handleSubmit((issue) => {
        createIssue(issue.title, issue.description);
    });

    // getActions gets the appropriate <Card.Actions> for an issue based on its status
    const getActions = (issue: Issue) => {
        if(issue.spec.status === 'open') {
            return (
                <Card.Actions>
                    <Button key="mark-in-progress" onClick={() => {updateStatus(issue, 'in_progress')}}>Start Progress</Button>
                </Card.Actions>
            )
        } else if(issue.spec.status === 'in_progress') {
            return (
                <Card.Actions>
                    <Button key="mark-open" onClick={() => {updateStatus(issue, 'open')}}>Stop Progress</Button>
                    <Button key="mark-closed" onClick={() => {updateStatus(issue, 'closed')}}>Complete</Button>
                </Card.Actions>
            )
        } else {
            return <Card.Actions></Card.Actions>
        }
    }

    return (
        <PluginPage>
            <div>
                <h1>Issue list</h1>
                {issuesData.length > 0 && (
                    <ul>
                        {issuesData.map((issue) => (
                            <li key={issue.metadata.name}>
                                <Card>
                                    <Card.Heading>{issue.spec.title}</Card.Heading>
                                    <Card.Description>{issue.spec.description}</Card.Description>
                                    <Card.Tags>
                                        <TagList tags={[issue.spec.status]} />
                                    </Card.Tags>
                                    { getActions(issue) }
                                    <Card.SecondaryActions>
                                        <IconButton
                                            key="delete-issue"
                                            name="trash-alt"
                                            size={'md'}
                                            aria-label="delete-issue"
                                            onClick={() => {
                                                deleteIssue(issue.metadata.name);
                                            }}
                                        >
                                            Delete
                                        </IconButton>
                                    </Card.SecondaryActions>
                                </Card>
                            </li>
                        ))}
                    </ul>
                )}
                <br />
                <h1>Create New Issue</h1>
                <form onSubmit={handleCreate}>
                    <Field label="Issue Title">
                        <Input type="text" aria-label="issue title" id="title" {...register('title')} />
                    </Field>
                    <Field label="Issue Description">
                        <Input type="text" aria-label="issue description" id="description" {...register('description')} />
                    </Field>
                    <Button type="submit" aria-label="Create Issue">
                        Create
                    </Button>
                </form>
            </div>
        </PluginPage>
    );
}

export default PageOne;
