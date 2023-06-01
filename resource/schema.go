package resource

import (
	"fmt"
	"reflect"
	"strings"
)

type SchemaScope string

const (
	NamespacedScope = SchemaScope("Namespaced")
	ClusterScope    = SchemaScope("Clustered")
)

// Schema represents a schema object
type Schema interface {
	// Group returns the Schema group
	Group() string
	// Version returns the Schema version
	Version() string
	// Kind returns the Schema kind
	Kind() string
	// Plural returns the plural name of the Schema kind
	Plural() string
	// ZeroValue returns the "zero-value", "default", or "empty" version of an Object of this Schema
	ZeroValue() Object
	// Scope returns the scope of the schema object
	Scope() SchemaScope
}

// SchemaGroup represents a group of Schemas. The interface does not require commonality between Schemas,
// but an implementation may require a relationship.
type SchemaGroup interface {
	Schemas() []Schema
}

// SimpleSchema is a simple implementation of Schema. It can be used for constructing simple Schemas,
// though the easiest way to define a schema is via codegen.
// TODO: codegen info
type SimpleSchema struct {
	group   string
	version string
	kind    string
	plural  string
	scope   SchemaScope
	zero    Object
}

// Group returns the SimpleSchema's Group
func (s *SimpleSchema) Group() string {
	return s.group
}

// Version returns the SimpleSchema's Version
func (s *SimpleSchema) Version() string {
	return s.version
}

// Kind returns the SimpleSchema's Kind
func (s *SimpleSchema) Kind() string {
	return s.kind
}

// Plural returns the SimpleSchema's Plural
func (s *SimpleSchema) Plural() string {
	return s.plural
}

// Scope returns the SimpleSchema's Scope
func (s *SimpleSchema) Scope() SchemaScope {
	return s.scope
}

// ZeroValue returns a copy the SimpleSchema's zero-valued Object instance
// It can be used directly, as the returned interface is a copy.
func (s *SimpleSchema) ZeroValue() Object {
	return s.zero.Copy()
}

// SimpleSchemaGroup collects schemas with the same group and version
type SimpleSchemaGroup struct {
	group   string
	version string
	schemas []Schema
}

// Schemas returns the SimpleSchemaGroup's list of Schemas
func (g *SimpleSchemaGroup) Schemas() []Schema {
	return g.schemas
}

// AddSchema creates a new SimpleSchema with the SimpleSchemaGroup's group and version,
// adds it to the SimpleSchemaGroup, and returns the created SimpleSchema
func (g *SimpleSchemaGroup) AddSchema(zeroVal Object, opts ...SimpleSchemaOption) *SimpleSchema {
	s := NewSimpleSchema(g.group, g.version, zeroVal, opts...)
	g.schemas = append(g.schemas, s)
	return s
}

// SimpleSchemaOption is an options function that can be passed to NewSimpleSchema to modify the resulting output
type SimpleSchemaOption func(*SimpleSchema)

// WithPlural returns a SimpleSchemaOption that sets the SimpleSchema's Plural to the provided string
func WithPlural(plural string) func(*SimpleSchema) {
	return func(s *SimpleSchema) {
		s.plural = plural
	}
}

// WithKind returns a SimpleSchemaOption that sets the SimpleSchema's Kind to the provided string
// TODO: still unsure on whether kind should be optional?
// It feels non-idiomatic to grab the kind from the reflected type name
func WithKind(kind string) func(*SimpleSchema) {
	return func(s *SimpleSchema) {
		s.kind = kind
	}
}

// WithScope returns a SimpleSchemaOption that sets the SimpleSchema's Scope to the provided SchemaScope
func WithScope(scope SchemaScope) func(schema *SimpleSchema) {
	return func(s *SimpleSchema) {
		s.scope = scope
	}
}

// NewSimpleSchema returns a new SimpleSchema
func NewSimpleSchema(group, version string, zeroVal Object, opts ...SimpleSchemaOption) *SimpleSchema {
	s := SimpleSchema{
		group:   group,
		version: version,
		zero:    zeroVal,
	}
	for _, opt := range opts {
		opt(&s)
	}
	if s.kind == "" {
		t := reflect.TypeOf(zeroVal)
		for t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		s.kind = t.Name()
	}
	if s.scope == "" {
		s.scope = NamespacedScope
	}
	if s.plural == "" {
		s.plural = fmt.Sprintf("%ss", strings.ToLower(s.kind))
	}
	return &s
}

// NewSimpleSchemaGroup returns a new SimpleSchemaGroup
func NewSimpleSchemaGroup(group, version string) *SimpleSchemaGroup {
	return &SimpleSchemaGroup{
		group:   group,
		version: version,
		schemas: make([]Schema, 0),
	}
}
