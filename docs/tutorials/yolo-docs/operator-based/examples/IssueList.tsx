import React, { useEffect, useState } from 'react';

interface Issue {
  metadata: {
    name: string;
    namespace: string;
    creationTimestamp?: string;
  };
  spec: {
    title: string;
    description: string;
    status: string;
    flagged?: boolean;
  };
}

export function IssueList() {
  const [issues, setIssues] = useState<Issue[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    // Fetch issues from the API
    fetch(`/apis/issuetrackerproject.ext.grafana.com/v1alpha1/namespaces/default/issues`, {
      credentials: 'include',
    })
      .then((res) => {
        if (!res.ok) {
          throw new Error(`HTTP error! status: ${res.status}`);
        }
        return res.json();
      })
      .then((data) => {
        setIssues(data.items || []);
        setLoading(false);
      })
      .catch((err) => {
        console.error('Failed to fetch issues:', err);
        setError(err.message);
        setLoading(false);
      });
  }, []);

  if (loading) {
    return <div>Loading issues...</div>;
  }

  if (error) {
    return <div>Error loading issues: {error}</div>;
  }

  if (issues.length === 0) {
    return (
      <div>
        <h2>Issues</h2>
        <p>No issues found. Create one using the API or kubectl.</p>
      </div>
    );
  }

  return (
    <div>
      <h2>Issues</h2>
      <ul style={{ listStyle: 'none', padding: 0 }}>
        {issues.map((issue) => (
          <li
            key={`${issue.metadata.namespace}/${issue.metadata.name}`}
            style={{
              padding: '10px',
              marginBottom: '10px',
              border: '1px solid #ddd',
              borderRadius: '4px',
            }}
          >
            <div>
              {/* Show flag icon if issue is flagged */}
              {issue.spec.flagged && (
                <span
                  title="Flagged as urgent by operator"
                  style={{ marginRight: '8px', fontSize: '1.2em' }}
                >
                  🚩
                </span>
              )}
              <strong>{issue.spec.title}</strong>
              <span style={{ marginLeft: '10px', color: '#666' }}>
                ({issue.spec.status})
              </span>
            </div>
            <div style={{ marginTop: '5px', fontSize: '0.9em', color: '#666' }}>
              {issue.spec.description}
            </div>
            <div style={{ marginTop: '5px', fontSize: '0.8em', color: '#999' }}>
              {issue.metadata.namespace}/{issue.metadata.name}
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}

