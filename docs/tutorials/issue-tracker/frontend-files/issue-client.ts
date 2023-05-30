import { Issue as generatedIssue } from '../generated/issue_types.gen';
import { BackendSrvRequest, getBackendSrv, FetchResponse } from '@grafana/runtime';
import { lastValueFrom } from 'rxjs';
import { PLUGIN_API_URL } from '../constants';

export interface ListResponse<T> {
    items: T[];
}

export interface Issue extends generatedIssue {
    staticMetadata: {
        name: string
        namespace: string
    }
}

export class IssueClient {
    apiEndpoint: string

    constructor() {
        this.apiEndpoint = PLUGIN_API_URL + '/issues';
    }

    async create(title: string, description: string): Promise<FetchResponse<Issue>> {
        let issue = {
            spec: {
                title: title,
                description: description,
                status: 'open',
            },
            staticMetadata: {
                name: 'issue-' + makeid(10),
                namespace: 'default',
            }
        }
        const options: BackendSrvRequest = {
            headers: {},
            method: 'POST',
            url: this.apiEndpoint,
            data: JSON.stringify(issue),
            showErrorAlert: false,
        };
        return lastValueFrom(getBackendSrv().fetch<Issue>(options));
    }

    async get(name: string): Promise<FetchResponse<Issue>>    {
        const options: BackendSrvRequest = {
            headers: {},
            url: this.apiEndpoint + '/' + name,
            showErrorAlert: false,
        };
        return lastValueFrom(getBackendSrv().fetch<Issue>(options));
    }

    async list(filters?: string[]): Promise<FetchResponse<ListResponse<Issue>>> {
        const options: BackendSrvRequest = {
            headers: {},
            url: this.apiEndpoint,
            showErrorAlert: false,
        };
        if (filters !== undefined && filters !== null && filters.length > 0) {
            options.params = {
                'filters': filters.join(','),
            };
        }
        return lastValueFrom(getBackendSrv().fetch<ListResponse<Issue>>(options));
    }

    async delete(name: string): Promise<FetchResponse<void>> {
        const options: BackendSrvRequest = {
            headers: {},
            method: 'DELETE',
            url: this.apiEndpoint + '/' + name,
            showErrorAlert: false,
        };
        return lastValueFrom(getBackendSrv().fetch<void>(options));
    }

    async update(name: string, updated: Issue): Promise<FetchResponse<Issue>> {
        const options: BackendSrvRequest = {
            headers: {},
            method: 'PUT',
            url: this.apiEndpoint + '/' + name,
            data: JSON.stringify(updated),
            showErrorAlert: false,
        };
        return lastValueFrom(getBackendSrv().fetch<Issue>(options));
    }
}

function makeid(length: number) {
    let result = '';
    const characters = 'abcdefghijklmnopqrstuvwxyz0123456789';
    const charactersLength = characters.length;
    let counter = 0;
    while (counter < length) {
        result += characters.charAt(Math.floor(Math.random() * charactersLength));
        counter += 1;
    }
    return result;
}

