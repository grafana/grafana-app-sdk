// Code generated - EDITING IS FUTILE. DO NOT EDIT.

// spec is the schema of our resource.
// We could include `status` or `metadata` top-level fields here as well,
// but `status` is for state information, which we don't need to track,
// and `metadata` is for kind/schema-specific custom metadata in addition to the existing
// common metadata, and we don't need to track any specific custom metadata.
export interface Spec {
	title: string;
	description: string;
	status: string;
}

export const defaultSpec = (): Spec => ({
	title: "",
	description: "",
	status: "",
});

