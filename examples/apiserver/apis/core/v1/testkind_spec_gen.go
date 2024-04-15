package v1

// TestKindSpec defines model for TestKindSpec.
// +k8s:openapi-gen=true
type TestKindSpec struct {
	StringField  string          `json:"stringField"`
	SubtypeField TestKindSubType `json:"subtypeField"`
}

// TestKindSubType defines model for TestKindSubType.
// +k8s:openapi-gen=true
type TestKindSubType struct {
	SubField1 int64 `json:"subField1"`
	SubField2 bool  `json:"subField2"`
}
