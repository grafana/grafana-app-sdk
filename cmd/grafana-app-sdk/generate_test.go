package main

import (
	"strings"
	"testing"

	"github.com/grafana/codejen"
)

func TestValidateGeneratedGoDecls(t *testing.T) {
	tests := []struct {
		name      string
		files     codejen.Files
		wantErr   bool
		wantInErr []string
	}{
		{
			name: "no collisions",
			files: codejen.Files{
				{RelativePath: "pkg/generated/foo/v1/foo_spec_gen.go", Data: []byte("package v1\ntype Spec struct{}\nfunc NewSpec() *Spec { return &Spec{} }\n")},
				{RelativePath: "pkg/generated/foo/v1/foo_status_gen.go", Data: []byte("package v1\ntype Status struct{}\nfunc NewStatus() *Status { return &Status{} }\n")},
			},
			wantErr: false,
		},
		{
			name: "type and constructor redeclared across files (issue #1043)",
			files: codejen.Files{
				{RelativePath: "pkg/generated/foo/v1/foo_spec_gen.go", Data: []byte("package v1\ntype Status struct{}\nfunc NewStatus() *Status { return &Status{} }\n")},
				{RelativePath: "pkg/generated/foo/v1/foo_status_gen.go", Data: []byte("package v1\ntype Status struct{}\nfunc NewStatus() *Status { return &Status{} }\n")},
			},
			wantErr:   true,
			wantInErr: []string{"Status", "NewStatus", "foo_spec_gen.go", "foo_status_gen.go"},
		},
		{
			name: "same identifier in different packages is fine",
			files: codejen.Files{
				{RelativePath: "pkg/generated/foo/v1/foo_status_gen.go", Data: []byte("package v1\ntype Status struct{}\n")},
				{RelativePath: "pkg/generated/bar/v1/bar_status_gen.go", Data: []byte("package v1\ntype Status struct{}\n")},
			},
			wantErr: false,
		},
		{
			name: "methods with the same name on different types do not collide",
			files: codejen.Files{
				{RelativePath: "pkg/generated/foo/v1/foo_spec_gen.go", Data: []byte("package v1\ntype Spec struct{}\nfunc (Spec) OpenAPIModelName() string { return \"\" }\n")},
				{RelativePath: "pkg/generated/foo/v1/foo_status_gen.go", Data: []byte("package v1\ntype Status struct{}\nfunc (Status) OpenAPIModelName() string { return \"\" }\n")},
			},
			wantErr: false,
		},
		{
			name: "non-go files are ignored",
			files: codejen.Files{
				{RelativePath: "definitions/foo.json", Data: []byte("{ not go }")},
			},
			wantErr: false,
		},
		{
			name: "a const colliding with a type is detected",
			files: codejen.Files{
				{RelativePath: "pkg/generated/foo/v1/foo_status_gen.go", Data: []byte("package v1\ntype Status struct{}\n")},
				{RelativePath: "pkg/generated/foo/v1/foo_spec_gen.go", Data: []byte("package v1\nconst Status = \"x\"\n")},
			},
			wantErr:   true,
			wantInErr: []string{"Status"},
		},
		{
			name: "repeated blank identifier vars do not collide",
			files: codejen.Files{
				{RelativePath: "pkg/generated/foo/v1/foo_object_gen.go", Data: []byte("package v1\ntype Corpus struct{}\nvar _ = Corpus{}\n")},
				{RelativePath: "pkg/generated/foo/v1/foo_client_gen.go", Data: []byte("package v1\ntype Client struct{}\nvar _ = Client{}\n")},
			},
			wantErr: false,
		},
		{
			name: "repeated init funcs do not collide",
			files: codejen.Files{
				{RelativePath: "pkg/generated/foo/v1/foo_a_gen.go", Data: []byte("package v1\nfunc init() {}\n")},
				{RelativePath: "pkg/generated/foo/v1/foo_b_gen.go", Data: []byte("package v1\nfunc init() {}\n")},
			},
			wantErr: false,
		},
		{
			name: "duplicate names within a single file are detected",
			files: codejen.Files{
				{RelativePath: "pkg/generated/foo/v1/foo_spec_gen.go", Data: []byte("package v1\ntype Status struct{}\nconst Status = \"x\"\n")},
			},
			wantErr:   true,
			wantInErr: []string{"Status", "foo_spec_gen.go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGeneratedGoDecls(tt.files)
			if tt.wantErr && err == nil {
				t.Fatalf("expected an error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			for _, want := range tt.wantInErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err.Error(), want)
				}
			}
		})
	}
}
