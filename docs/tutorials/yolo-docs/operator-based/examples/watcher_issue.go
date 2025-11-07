package watchers

import (
	"context"
	"fmt"
	"strings"

	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
	"go.opentelemetry.io/otel"

	issuev1alpha1 "github.com/grafana/issue-tracker-project/pkg/generated/resource/issue/v1alpha1"
)

var _ operator.ResourceWatcher = &IssueWatcher{}

type IssueWatcher struct {
	client resource.Client
}

func NewIssueWatcher(client resource.Client) (*IssueWatcher, error) {
	return &IssueWatcher{client: client}, nil
}

// Add handles add events for issuev1alpha1.Issue resources.
// It checks if the issue description contains urgent keywords and flags it if needed.
func (w *IssueWatcher) Add(ctx context.Context, rObj resource.Object) error {
	ctx, span := otel.GetTracerProvider().Tracer("watcher").Start(ctx, "watcher-add")
	defer span.End()

	issue, ok := rObj.(*issuev1alpha1.Issue)
	if !ok {
		return fmt.Errorf("provided object is not of type *issuev1alpha1.Issue (name=%s, namespace=%s, kind=%s)",
			rObj.GetStaticMetadata().Name, rObj.GetStaticMetadata().Namespace, rObj.GetStaticMetadata().Kind)
	}

	// Check if description contains urgent keywords
	if containsUrgentKeyword(issue.Spec.Description) && !issue.Spec.Flagged {
		logging.FromContext(ctx).Info("Flagging issue with urgent keyword",
			"name", issue.GetName(),
			"namespace", issue.GetNamespace())

		// Set the flagged field
		issue.Spec.Flagged = true

		// Update the issue
		_, err := w.client.Update(ctx, issue.GetStaticMetadata().Identifier(), issue)
		if err != nil {
			return fmt.Errorf("failed to flag issue: %w", err)
		}

		logging.FromContext(ctx).Info("Successfully flagged issue", "name", issue.GetName())
	} else {
		logging.FromContext(ctx).Debug("Issue does not contain urgent keywords or is already flagged",
			"name", issue.GetName(),
			"flagged", issue.Spec.Flagged)
	}

	return nil
}

// Update handles update events for issuev1alpha1.Issue resources.
// For this simple example, we use the same logic as Add to check if the updated description
// contains urgent keywords.
func (w *IssueWatcher) Update(ctx context.Context, rOld resource.Object, rNew resource.Object) error {
	ctx, span := otel.GetTracerProvider().Tracer("watcher").Start(ctx, "watcher-update")
	defer span.End()

	oldIssue, ok := rOld.(*issuev1alpha1.Issue)
	if !ok {
		return fmt.Errorf("provided old object is not of type *issuev1alpha1.Issue (name=%s, namespace=%s, kind=%s)",
			rOld.GetStaticMetadata().Name, rOld.GetStaticMetadata().Namespace, rOld.GetStaticMetadata().Kind)
	}

	newIssue, ok := rNew.(*issuev1alpha1.Issue)
	if !ok {
		return fmt.Errorf("provided new object is not of type *issuev1alpha1.Issue (name=%s, namespace=%s, kind=%s)",
			rNew.GetStaticMetadata().Name, rNew.GetStaticMetadata().Namespace, rNew.GetStaticMetadata().Kind)
	}

	// Only process if the description changed
	if oldIssue.Spec.Description != newIssue.Spec.Description {
		logging.FromContext(ctx).Debug("Issue description changed, re-checking for urgent keywords",
			"name", newIssue.GetName())
		return w.Add(ctx, rNew)
	}

	logging.FromContext(ctx).Debug("Issue updated but description unchanged",
		"name", newIssue.GetName())
	return nil
}

// Delete handles delete events for issuev1alpha1.Issue resources.
// No cleanup is needed for our simple flagging logic.
func (w *IssueWatcher) Delete(ctx context.Context, rObj resource.Object) error {
	ctx, span := otel.GetTracerProvider().Tracer("watcher").Start(ctx, "watcher-delete")
	defer span.End()

	issue, ok := rObj.(*issuev1alpha1.Issue)
	if !ok {
		return fmt.Errorf("provided object is not of type *issuev1alpha1.Issue (name=%s, namespace=%s, kind=%s)",
			rObj.GetStaticMetadata().Name, rObj.GetStaticMetadata().Namespace, rObj.GetStaticMetadata().Kind)
	}

	logging.FromContext(ctx).Debug("Issue deleted",
		"name", issue.GetStaticMetadata().Name,
		"namespace", issue.GetStaticMetadata().Namespace)
	return nil
}

// Sync is not a standard resource.Watcher function, but is used when wrapping this watcher in an operator.OpinionatedWatcher.
// It handles resources which MAY have been updated during an outage period where the watcher was not able to consume events.
func (w *IssueWatcher) Sync(ctx context.Context, rObj resource.Object) error {
	ctx, span := otel.GetTracerProvider().Tracer("watcher").Start(ctx, "watcher-sync")
	defer span.End()

	issue, ok := rObj.(*issuev1alpha1.Issue)
	if !ok {
		return fmt.Errorf("provided object is not of type *issuev1alpha1.Issue (name=%s, namespace=%s, kind=%s)",
			rObj.GetStaticMetadata().Name, rObj.GetStaticMetadata().Namespace, rObj.GetStaticMetadata().Kind)
	}

	logging.FromContext(ctx).Debug("Syncing issue (possible update during downtime)",
		"name", issue.GetStaticMetadata().Name)

	// Treat sync the same as Add - re-check if it should be flagged
	return w.Add(ctx, rObj)
}

// containsUrgentKeyword checks if the given text contains any of the urgent keywords.
// Keywords are matched case-insensitively.
func containsUrgentKeyword(text string) bool {
	keywords := []string{"URGENT", "CRITICAL", "ASAP", "EMERGENCY"}
	upper := strings.ToUpper(text)
	for _, keyword := range keywords {
		if strings.Contains(upper, keyword) {
			return true
		}
	}
	return false
}

