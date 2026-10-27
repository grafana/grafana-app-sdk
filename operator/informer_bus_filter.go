package operator

import (
	"fmt"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/grafana/grafana-app-sdk/resource"
)

// objectMatcher decides whether an object is inside the scope an informer was configured for. It is what lets a
// filtered informer take its live events from a source which cannot filter: the list is narrowed by the server, and
// everything the live stream delivers is checked here against the same selectors.
//
// A nil matcher accepts everything, so an unfiltered informer pays nothing.
type objectMatcher struct {
	labels labels.Selector
	fields fields.Selector
	// fieldValues resolves the selectable fields the field selector names, by field path.
	fieldValues map[string]func(resource.Object) (string, error)
}

// newObjectMatcher builds the matcher for a kind and its filters, or nil when there are none. It fails rather than
// ignoring anything it cannot evaluate: a selector silently dropped would hand the informer objects the app never
// asked to watch.
//
// payload says what the delivered objects carry. An identity payload carries only what the bus announced, so a
// label selector needs every key it reads to be announced and a field selector cannot be evaluated at all.
func newObjectMatcher(kind resource.Kind, opts ListWatchOptions, payload BusPayload, announced AnnouncedMetadata) (*objectMatcher, error) {
	if len(opts.LabelFilters) == 0 && len(opts.FieldSelectors) == 0 {
		return nil, nil
	}
	m := &objectMatcher{}

	if len(opts.LabelFilters) > 0 {
		selector, err := labels.Parse(joinSelectors(opts.LabelFilters))
		if err != nil {
			return nil, fmt.Errorf("invalid label filters: %w", err)
		}
		if payload == PayloadIdentity {
			if err := announcedCoversLabels(selector, announced); err != nil {
				return nil, err
			}
		}
		m.labels = selector
	}

	if len(opts.FieldSelectors) > 0 {
		if payload == PayloadIdentity {
			return nil, fmt.Errorf("field selectors cannot be used with %s: a bus announces no object fields, so a live event cannot be matched against them", PayloadIdentity)
		}
		selector, err := fields.ParseSelector(joinSelectors(opts.FieldSelectors))
		if err != nil {
			return nil, fmt.Errorf("invalid field selectors: %w", err)
		}
		values, err := selectableFieldValues(kind, selector)
		if err != nil {
			return nil, err
		}
		m.fields, m.fieldValues = selector, values
	}
	return m, nil
}

// Matches reports whether obj is inside the configured scope. A matcher which cannot read what it needs off the
// object reports false, so an object is never delivered on the strength of a selector that was not evaluated.
func (m *objectMatcher) Matches(obj resource.Object) bool {
	if m == nil {
		return true
	}
	if obj == nil {
		return false
	}
	if m.labels != nil && !m.labels.Matches(labels.Set(obj.GetLabels())) {
		return false
	}
	if m.fields == nil {
		return true
	}
	set := fields.Set{}
	for path, value := range m.fieldValues {
		resolved, err := value(obj)
		if err != nil {
			return false
		}
		set[path] = resolved
	}
	return m.fields.Matches(set)
}

// announcedCoversLabels reports whether every label key the selector reads is announced. An announced metadata
// field which is absent means unknown rather than empty, so matching on a key the bus does not announce would read
// "not set" for an object which has it — admitting or excluding objects at random.
func announcedCoversLabels(selector labels.Selector, announced AnnouncedMetadata) error {
	requirements, _ := selector.Requirements()
	for _, requirement := range requirements {
		if !slices.Contains(announced.Labels, requirement.Key()) {
			return fmt.Errorf("label filter on %q cannot be used with %s: the bus does not announce that label, so a live event cannot be matched against it", requirement.Key(), PayloadIdentity)
		}
	}
	return nil
}

// selectableFieldValues resolves the kind's value function for each field the selector names. A field the kind does
// not declare selectable has no value function, so the selector could not be evaluated against a delivered object.
func selectableFieldValues(kind resource.Kind, selector fields.Selector) (map[string]func(resource.Object) (string, error), error) {
	declared := map[string]func(resource.Object) (string, error){}
	if kind.Schema != nil {
		for _, field := range kind.SelectableFields() {
			declared[field.FieldSelector] = field.FieldValueFunc
		}
	}
	out := map[string]func(resource.Object) (string, error){}
	for _, requirement := range selector.Requirements() {
		value, ok := declared[requirement.Field]
		if !ok || value == nil {
			return nil, fmt.Errorf("field selector on %q cannot be used: the kind does not declare it selectable, so a delivered object cannot be matched against it", requirement.Field)
		}
		out[requirement.Field] = value
	}
	return out, nil
}

// joinSelectors renders a set of selector strings as the single comma-separated expression the parsers take.
func joinSelectors(selectors []string) string {
	return strings.Join(selectors, ",")
}
