package templates

import (
	"embed"
	"fmt"
	"io"
	"regexp"
	"text/template"

	"github.com/grafana/grafana-app-sdk/codegen"
)

//go:embed *.tmpl plugin/*.tmpl secure/*.tmpl operator/*.tmpl
var templates embed.FS

var (
	templateResourceObject, _ = template.ParseFS(templates, "resourceobject.tmpl")
	templateSchema, _         = template.ParseFS(templates, "schema.tmpl")
	templateCodec, _          = template.ParseFS(templates, "codec.tmpl")
	templateLineage, _        = template.ParseFS(templates, "lineage.tmpl")
	templateThemaCodec, _     = template.ParseFS(templates, "themacodec.tmpl")
	templateWrappedType, _    = template.ParseFS(templates, "wrappedtype.tmpl")
	templateTSType, _         = template.ParseFS(templates, "tstype.tmpl")

	templateBackendPluginRouter, _          = template.ParseFS(templates, "plugin/plugin.tmpl")
	templateBackendPluginResourceHandler, _ = template.ParseFS(templates, "plugin/handler_resource.tmpl")
	templateBackendPluginModelsHandler, _   = template.ParseFS(templates, "plugin/handler_models.tmpl")
	templateBackendMain, _                  = template.ParseFS(templates, "plugin/main.tmpl")

	templateWatcher, _            = template.ParseFS(templates, "operator/watcher.tmpl")
	templateOperatorKubeconfig, _ = template.ParseFS(templates, "operator/kubeconfig.tmpl")
	templateOperatorMain, _       = template.ParseFS(templates, "operator/main.tmpl")
	templateOperatorConfig, _     = template.ParseFS(templates, "operator/config.tmpl")
)

var (
	// GoTypeString is a CustomMetadataFieldGoType for "string" go types
	GoTypeString = CustomMetadataFieldGoType{
		GoType: "string",
		SetFuncTemplate: func(inputVarName string, setToVarName string) string {
			return fmt.Sprintf("%s = %s", setToVarName, inputVarName)
		},
		GetFuncTemplate: func(varName string) string {
			return fmt.Sprintf("return %s", varName)
		},
	}
	// GoTypeInt is a CustomMetadataFieldGoType for "int" go types
	GoTypeInt = CustomMetadataFieldGoType{
		GoType: "int",
		SetFuncTemplate: func(inputVarName string, setToVarName string) string {
			return fmt.Sprintf("%s = strconv.Itoa(%s)", setToVarName, inputVarName)
		},
		GetFuncTemplate: func(varName string) string {
			return fmt.Sprintf("i, _ := strconv.Atoi(%s)\nreturn i", varName)
		},
		AdditionalImports: []string{"strconv"},
	}
	// GoTypeTime is a CustomMetadataFieldGoType for "time.Time" go types
	GoTypeTime = CustomMetadataFieldGoType{
		GoType: "time.Time",
		SetFuncTemplate: func(inputVarName string, setToVarName string) string {
			return fmt.Sprintf("%s = %s.Format(time.RFC3339)", setToVarName, inputVarName)
		},
		GetFuncTemplate: func(varName string) string {
			return fmt.Sprintf("parsed, _ := time.Parse(%s, time.RFC3339)\nreturn parsed", varName)
		},
		AdditionalImports: []string{"time"},
	}
)

// ResourceObjectTemplateMetadata is the metadata required by the Resource Object template
type ResourceObjectTemplateMetadata struct {
	Package              string
	TypeName             string
	SpecTypeName         string
	ObjectTypeName       string
	ObjectShortName      string
	Subresources         []SubresourceMetadata
	CustomMetadataFields []ObjectMetadataField
}

// SubresourceMetadata is subresource information used in templates
type SubresourceMetadata struct {
	TypeName string
	JSONName string
	Comment  string
}

// WriteResourceObject executes the Resource Object template, and writes out the generated go code to out
func WriteResourceObject(metadata ResourceObjectTemplateMetadata, out io.Writer) error {
	return templateResourceObject.Execute(out, metadata)
}

type ResourceTSTemplateMetadata struct {
	TypeName     string
	FilePrefix   string
	Subresources []SubresourceMetadata
}

func WriteResourceTSType(metadata ResourceTSTemplateMetadata, out io.Writer) error {
	return templateTSType.Execute(out, metadata)
}

// SchemaMetadata is the metadata required by the Resource Schema template
type SchemaMetadata struct {
	Package string
	Group   string
	Version string
	Kind    string
	Plural  string
	Scope   string
}

// WriteSchema executes the Resource Schema template, and writes out the generated go code to out
func WriteSchema(metadata SchemaMetadata, out io.Writer) error {
	return templateSchema.Execute(out, metadata)
}

// WriteCodec executes the Generic Resource Codec template, and writes out the generated go code to out
func WriteCodec(metadata SchemaMetadata, out io.Writer) error {
	return templateCodec.Execute(out, metadata)
}

// WriteThemaCodec executes the Thema-specific Codec template, and writes out the generated go code to out
func WriteThemaCodec(metadata ResourceObjectTemplateMetadata, out io.Writer) error {
	return templateThemaCodec.Execute(out, metadata)
}

// LineageMetadata is the metadata required by the lineage go code template
type LineageMetadata struct {
	Package           string
	TypeName          string
	CUEFile           string
	CUESelector       string
	SchemaPackagePath string
	SchemaPackageName string
	ObjectTypeName    string
	Subresources      []SubresourceMetadata
}

// WriteLineageGo executes the lineage go template, and writes out the generated go code to out
func WriteLineageGo(metadata LineageMetadata, out io.Writer) error {
	return templateLineage.Execute(out, metadata)
}

type ObjectMetadataField struct {
	JSONName  string
	FieldName string
	GoType    CustomMetadataFieldGoType
}

// CustomMetadataFieldGoType is a struct that contains information and codegen functions for a Go type which needs
// setters and getters in a resource.Object implementation
// TODO: do we need the approach to be this generic/is there a less-confusing way to implement this?
type CustomMetadataFieldGoType struct {
	// GoType is the type string (ex. "string", "time.Time")
	GoType string
	// SetFuncTemplate should return a go template string that sets the value.
	// inputVarName is the name of the variable in the SetX function (which has type of GoType),
	// and setToVarName is the name of the string-type variable which it must be set to
	SetFuncTemplate func(inputVarName string, setToVarName string) string
	// GetFuncTemplate should return a go template string that gets the value as the appropriate go type.
	// fromStringVarName is the string-type variable name which the value is currently stored as
	GetFuncTemplate func(fromStringVarName string) string
	// AdditionalImports is a list of any additional imports needed for the Get/Set functions or type (such as "time")
	AdditionalImports []string
}

// WrappedTypeMetadata is the metadata required by the wrappedtype go code template
type WrappedTypeMetadata struct {
	Package     string
	TypeName    string
	CUEFile     string
	CUESelector string
}

// WriteWrappedType executes the wrappedtype go template, and writes out the generated go code to out
func WriteWrappedType(metadata WrappedTypeMetadata, out io.Writer) error {
	return templateWrappedType.Execute(out, metadata)
}

// BackendPluginRouterTemplateMetadata is the metadata required by the Backend Plugin Router template
type BackendPluginRouterTemplateMetadata struct {
	Repo                  string
	APICodegenPath        string
	Resources             []codegen.KindProperties
	PluginID              string
	ResourcesAreVersioned bool
}

func (BackendPluginRouterTemplateMetadata) ToPackageName(input string) string {
	return ToPackageName(input)
}

// WriteBackendPluginRouter executes the Backend Plugin Router template, and writes out the generated go code to out
func WriteBackendPluginRouter(metadata BackendPluginRouterTemplateMetadata, out io.Writer) error {
	return templateBackendPluginRouter.Execute(out, metadata)
}

// BackendPluginHandlerTemplateMetadata is the metadata required by the Backend Plugin Handler template
type BackendPluginHandlerTemplateMetadata struct {
	codegen.KindProperties
	Repo           string
	APICodegenPath string
	TypeName       string
	IsResource     bool
	Version        string
}

func (BackendPluginHandlerTemplateMetadata) ToPackageName(input string) string {
	return ToPackageName(input)
}

// WriteBackendPluginHandler executes the Backend Plugin Handler template, and writes out the generated go code to out
func WriteBackendPluginHandler(metadata BackendPluginHandlerTemplateMetadata, out io.Writer) error {
	// Easier to have two templates than deal with all the if...else statements in the template
	if !metadata.IsResource {
		return templateBackendPluginModelsHandler.Execute(out, metadata)
	}
	return templateBackendPluginResourceHandler.Execute(out, metadata)
}

// WriteBackendPluginMain executes the Backend Plugin Main template, and writes out the generated go code to out
func WriteBackendPluginMain(metadata BackendPluginRouterTemplateMetadata, out io.Writer) error {
	return templateBackendMain.Execute(out, metadata)
}

// GetBackendPluginSecurePackageFiles returns go files for the `secure` package in the backend plugin, as a map of
// <filename> (without "secure" in path) => contents
func GetBackendPluginSecurePackageFiles() (map[string][]byte, error) {
	dataF, err := templates.ReadFile("secure/data.tmpl")
	if err != nil {
		return nil, err
	}
	middlewareF, err := templates.ReadFile("secure/middleware.tmpl")
	if err != nil {
		return nil, err
	}
	retrieverF, err := templates.ReadFile("secure/retriever.tmpl")
	if err != nil {
		return nil, err
	}
	return map[string][]byte{
		"data.go":       dataF,
		"middleware.go": middlewareF,
		"retriever.go":  retrieverF,
	}, nil
}

type WatcherMetadata struct {
	codegen.KindProperties
	PackageName string
	Repo        string
	CodegenPath string
	Version     string
}

func (WatcherMetadata) ToPackageName(input string) string {
	return ToPackageName(input)
}

func WriteWatcher(metadata WatcherMetadata, out io.Writer) error {
	return templateWatcher.Execute(out, metadata)
}

func WriteOperatorKubeConfig(out io.Writer) error {
	return templateOperatorKubeconfig.Execute(out, nil)
}

type OperatorMainMetadata struct {
	PackageName           string
	ProjectName           string
	Repo                  string
	CodegenPath           string
	WatcherPackage        string
	ResourcesAreVersioned bool
	Resources             []codegen.KindProperties
}

func (OperatorMainMetadata) ToPackageName(input string) string {
	return ToPackageName(input)
}

func WriteOperatorMain(metadata OperatorMainMetadata, out io.Writer) error {
	return templateOperatorMain.Execute(out, metadata)
}

func WriteOperatorConfig(out io.Writer) error {
	return templateOperatorConfig.Execute(out, nil)
}

// ToPackageName sanitizes an input into a deterministic allowed go package name.
// It is used to turn kind names or versions into package names when performing go code generation.
func ToPackageName(input string) string {
	return regexp.MustCompile(`([^A-Za-z0-9_])`).ReplaceAllString(input, "_")
}
