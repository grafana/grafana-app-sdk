package templates

import (
	"embed"
	"io"
	"text/template"

	"github.com/grafana/kindsys"
)

//go:embed *.tmpl plugin/*.tmpl secure/*.tmpl operator/*.tmpl
var templates embed.FS

var (
	templateResourceObject, _ = template.ParseFS(templates, "resourceobject.tmpl")
	templateSchema, _         = template.ParseFS(templates, "schema.tmpl")
	templateLineage, _        = template.ParseFS(templates, "lineage.tmpl")
	templateWrappedType, _    = template.ParseFS(templates, "wrappedtype.tmpl")

	templateBackendPluginRouter, _          = template.ParseFS(templates, "plugin/plugin.tmpl")
	templateBackendPluginResourceHandler, _ = template.ParseFS(templates, "plugin/handler_resource.tmpl")
	templateBackendPluginModelsHandler, _   = template.ParseFS(templates, "plugin/handler_models.tmpl")
	templateBackendMain, _                  = template.ParseFS(templates, "plugin/main.tmpl")

	templateWatcher, _            = template.ParseFS(templates, "operator/watcher.tmpl")
	templateOperatorKubeconfig, _ = template.ParseFS(templates, "operator/kubeconfig.tmpl")
	templateOperatorMain, _       = template.ParseFS(templates, "operator/main.tmpl")
	templateOperatorConfig, _     = template.ParseFS(templates, "operator/config.tmpl")
	templateOperatorTelemetry, _  = template.ParseFS(templates, "operator/telemetry.tmpl")
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

// LineageMetadata is the metadata required by the lineage go code template
type LineageMetadata struct {
	Package           string
	TypeName          string
	CUEFile           string
	CUESelector       string
	SchemaPackagePath string
	SchemaPackageName string
	Subresources      []SubresourceMetadata
}

// WriteLineageGo executes the lineage go template, and writes out the generated go code to out
func WriteLineageGo(metadata LineageMetadata, out io.Writer) error {
	return templateLineage.Execute(out, metadata)
}

type ObjectMetadataField struct {
	JSONName  string
	FieldName string
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
	Repo           string
	APICodegenPath string
	Resources      []kindsys.CustomProperties
	PluginID       string
}

// WriteBackendPluginRouter executes the Backend Plugin Router template, and writes out the generated go code to out
func WriteBackendPluginRouter(metadata BackendPluginRouterTemplateMetadata, out io.Writer) error {
	return templateBackendPluginRouter.Execute(out, metadata)
}

// BackendPluginHandlerTemplateMetadata is the metadata required by the Backend Plugin Handler template
type BackendPluginHandlerTemplateMetadata struct {
	kindsys.CustomProperties
	Repo           string
	APICodegenPath string
	TypeName       string
	IsResource     bool
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
	kindsys.CustomProperties
	PackageName string
	Repo        string
	CodegenPath string
}

func WriteWatcher(metadata WatcherMetadata, out io.Writer) error {
	return templateWatcher.Execute(out, metadata)
}

func WriteOperatorKubeConfig(out io.Writer) error {
	return templateOperatorKubeconfig.Execute(out, nil)
}

type OperatorMainMetadata struct {
	PackageName    string
	Repo           string
	CodegenPath    string
	WatcherPackage string
	Resources      []kindsys.CustomProperties
}

func WriteOperatorMain(metadata OperatorMainMetadata, out io.Writer) error {
	return templateOperatorMain.Execute(out, metadata)
}

func WriteOperatorConfig(out io.Writer) error {
	return templateOperatorConfig.Execute(out, nil)
}

func WriteOperatorTelemetry(out io.Writer) error {
	return templateOperatorTelemetry.Execute(out, nil)
}
