// Package searchfields is the single source of truth for which search-field
// capabilities are valid on which field types.
//
// Both the SDK's codegen-time validator and Grafana's runtime validator
// consume this matrix so the two cannot drift. The package deliberately
// depends only on the standard library (no CUE or codegen imports) so that
// runtime consumers can import it without pulling in code-generation
// machinery.
package searchfields

import (
	"errors"
	"fmt"
	"slices"
)

// Field types. These mirror the values allowed by the manifest CUE schema's
// #SearchField.type and Grafana's SearchFieldType. TypeUnknown is the zero
// value used for column shapes that have no dedicated type.
const (
	TypeUnknown = ""
	TypeString  = "string"
	TypeInt64   = "int64"
	TypeDouble  = "double"
	TypeBoolean = "boolean"
	TypeDate    = "date"
)

// Capabilities identify what a search field can be used for at query time.
// These mirror the manifest CUE schema's #SearchField.capabilities and
// Grafana's SearchCapability.
const (
	// CapabilityFilter allows exact-match filtering on the field.
	CapabilityFilter = "filter"
	// CapabilityText allows full-token (full-text) search on the field.
	CapabilityText = "text"
	// CapabilityPartial allows substring matching. It requires CapabilityText.
	CapabilityPartial = "partial"
	// CapabilitySort makes the field sortable.
	CapabilitySort = "sort"
	// CapabilityFacet makes the field facetable (grouped counts by value).
	CapabilityFacet = "facet"
	// CapabilityRetrieve stores the value so it is returned in search results.
	CapabilityRetrieve = "retrieve"
	// CapabilityUnranked applies to text fields only. It drops per-term
	// frequency stats and field-length norms to save index space, at the cost
	// of ranking quality, for text that is indexed but never scored.
	CapabilityUnranked = "unranked"
)

// allowedTypes maps a capability to the field types it may be declared on.
//
// A capability that is absent from this map (filter, retrieve, unranked) is
// valid on any field type. The restricted capabilities are:
//
//   - text, partial, facet: rely on text or keyword analysis under the bleve
//     text engine and have no meaning on numeric or boolean fields, so they
//     require a string-typed field.
//   - sort: makes the field sortable. Supported on string, numeric, and
//     boolean fields.
var allowedTypes = map[string][]string{
	CapabilityText:    {TypeString},
	CapabilityPartial: {TypeString},
	CapabilityFacet:   {TypeString},
	CapabilitySort:    {TypeString, TypeInt64, TypeDouble, TypeBoolean},
}

// Validate reports whether every capability is allowed on the given field
// type. It returns a joined error with one clear message per violation, or
// nil when all capabilities are valid for the type.
func Validate(fieldType string, capabilities []string) error {
	var violations []error
	for _, c := range capabilities {
		types, restricted := allowedTypes[c]
		if !restricted {
			continue
		}
		if !slices.Contains(types, fieldType) {
			violations = append(violations, fmt.Errorf("capability %q is not supported for type %q", c, fieldType))
		}
	}
	return errors.Join(violations...)
}
