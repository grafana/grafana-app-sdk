package v1

// Spec defines model for Spec.
type Spec struct {
	StringField  string  `json:"stringField"`
	SubtypeField SubType `json:"subtypeField"`
}

// SubType defines model for SubType.
type SubType struct {
	SubField1 int64 `json:"subField1"`
	SubField2 bool  `json:"subField2"`
}
