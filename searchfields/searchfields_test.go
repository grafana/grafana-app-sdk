package searchfields

import "testing"

func TestValidate(t *testing.T) {
	tests := []struct {
		name         string
		fieldType    string
		capabilities []string
		wantErr      bool
	}{
		// sort is now allowed on string, numeric, and boolean types.
		{name: "sort on string", fieldType: TypeString, capabilities: []string{CapabilitySort}},
		{name: "sort on int64", fieldType: TypeInt64, capabilities: []string{CapabilitySort}},
		{name: "sort on double", fieldType: TypeDouble, capabilities: []string{CapabilitySort}},
		{name: "sort on boolean", fieldType: TypeBoolean, capabilities: []string{CapabilitySort}},
		{name: "sort on date", fieldType: TypeDate, capabilities: []string{CapabilitySort}, wantErr: true},
		{name: "sort on unknown", fieldType: TypeUnknown, capabilities: []string{CapabilitySort}, wantErr: true},

		// text, partial, facet require a string type.
		{name: "text on string", fieldType: TypeString, capabilities: []string{CapabilityText}},
		{name: "partial on string", fieldType: TypeString, capabilities: []string{CapabilityPartial}},
		{name: "facet on string", fieldType: TypeString, capabilities: []string{CapabilityFacet}},
		{name: "text on int64", fieldType: TypeInt64, capabilities: []string{CapabilityText}, wantErr: true},
		{name: "partial on double", fieldType: TypeDouble, capabilities: []string{CapabilityPartial}, wantErr: true},
		{name: "facet on boolean", fieldType: TypeBoolean, capabilities: []string{CapabilityFacet}, wantErr: true},
		{name: "text on date", fieldType: TypeDate, capabilities: []string{CapabilityText}, wantErr: true},

		// filter, retrieve, unranked are valid on any type.
		{name: "filter on int64", fieldType: TypeInt64, capabilities: []string{CapabilityFilter}},
		{name: "retrieve on boolean", fieldType: TypeBoolean, capabilities: []string{CapabilityRetrieve}},
		{name: "unranked on date", fieldType: TypeDate, capabilities: []string{CapabilityUnranked}},

		// representative valid combination.
		{name: "string with filter/text/sort", fieldType: TypeString, capabilities: []string{CapabilityFilter, CapabilityText, CapabilitySort}},
		// multiple violations at once.
		{name: "int64 with text and facet", fieldType: TypeInt64, capabilities: []string{CapabilityText, CapabilityFacet}, wantErr: true},
		// empty capabilities are always valid.
		{name: "no capabilities", fieldType: TypeInt64},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.fieldType, tt.capabilities)
			if tt.wantErr && err == nil {
				t.Fatalf("Validate(%q, %v) = nil, want error", tt.fieldType, tt.capabilities)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate(%q, %v) = %v, want nil", tt.fieldType, tt.capabilities, err)
			}
		})
	}
}
