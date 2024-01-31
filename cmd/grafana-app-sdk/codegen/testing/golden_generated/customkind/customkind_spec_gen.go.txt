package customkind

import (
	"time"
)

// Defines values for SpecEnum.
const (
	SpecEnumDefault SpecEnum = "default"
	SpecEnumVal1    SpecEnum = "val1"
	SpecEnumVal2    SpecEnum = "val2"
	SpecEnumVal3    SpecEnum = "val3"
	SpecEnumVal4    SpecEnum = "val4"
)

// InnerObject1 defines model for InnerObject1.
type InnerObject1 struct {
	InnerField1 string         `json:"innerField1"`
	InnerField2 []string       `json:"innerField2"`
	InnerField3 []InnerObject2 `json:"innerField3"`
}

// InnerObject2 defines model for InnerObject2.
type InnerObject2 struct {
	Details map[string]interface{} `json:"details"`
	Name    string                 `json:"name"`
}

// Spec defines model for Spec.
type Spec struct {
	BoolField  bool             `json:"boolField"`
	Enum       SpecEnum         `json:"enum"`
	Field1     string           `json:"field1"`
	FloatField float64          `json:"floatField"`
	I32        int              `json:"i32"`
	I64        int              `json:"i64"`
	Inner      InnerObject1     `json:"inner"`
	Map        map[string]Type2 `json:"map"`
	Timestamp  time.Time        `json:"timestamp"`
	Union      interface{}      `json:"union"`
}

// SpecEnum defines model for Spec.Enum.
type SpecEnum string

// Type1 defines model for Type1.
type Type1 struct {
	Group   string   `json:"group"`
	Options []string `json:"options,omitempty"`
}

// Type2 defines model for Type2.
type Type2 struct {
	Details map[string]interface{} `json:"details"`
	Group   string                 `json:"group"`
}
