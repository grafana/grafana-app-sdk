import * as React from 'react';
import { css } from '@emotion/css';
import { useForm } from 'react-hook-form';
import { GrafanaTheme2 } from '@grafana/data';
import { useStyles2, Button, IconButton, Field, Input, Card, TagList } from '@grafana/ui';
import { Issue, IssueClient } from '../api/issue_client';
import { useState, useEffect } from 'react';

// This is used for the create new issue form
type ReactHookFormProps = {
    title: string;
    description: string;
};


// MainPage is the main (and only) page of the plugin, where issues are listed, and can be created, updated, or deleted
export const MainPage = () => {
    useStyles2(getStyles);

    let issues: Issue[] = [];
    const [issuesData, setIssuesData] = useState(issues);
    useEffect(() => {
        const fetchData = async () => {
            const client = new IssueClient()
            const issues = await client.list();
            setIssuesData(issues.data.items);
        }

        fetchData().catch(console.error);
    }, []);

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
        await ic.update(issue.staticMetadata.name, issue);
        await listIssues();
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
    <div>
        <h1>Issue list</h1>
        {issuesData.length > 0 && (
        <ul>
            {issuesData.map((issue: any) => (
            <li key={issue.staticMetadata.name}>
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
                        deleteIssue(issue.staticMetadata.name);
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
    );
};


const getStyles = (theme: GrafanaTheme2) => ({
    marginTop: css`
    margin-top: ${theme.spacing(2)};
    `,
});
