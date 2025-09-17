package templates

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"text/template"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/kube-openapi/pkg/validation/spec"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/codegen"
)

//go:embed *.tmpl plugin/*.tmpl secure/*.tmpl operator/*.tmpl app/*.tmpl
var templates embed.FS

var (
	funcMap = template.FuncMap{
		"list": func(items ...any) []any {
			return items
		},
		"refString": func(ref spec.Ref) string {
			return ref.String()
		},
	}

	templateResourceObject, _   = template.ParseFS(templates, "resourceobject.tmpl")
	templateSchema, _           = template.ParseFS(templates, "schema.tmpl")
	templateCodec, _            = template.ParseFS(templates, "codec.tmpl")
	templateLineage, _          = template.ParseFS(templates, "lineage.tmpl")
	templateThemaCodec, _       = template.ParseFS(templates, "themacodec.tmpl")
	templateWrappedType, _      = template.ParseFS(templates, "wrappedtype.tmpl")
	templateTSType, _           = template.ParseFS(templates, "tstype.tmpl")
	templateConstants, _        = template.ParseFS(templates, "constants.tmpl")
	templateGoResourceClient, _ = template.ParseFS(templates, "resourceclient.tmpl")
	templateRuntimeObject, _    = template.ParseFS(templates, "runtimeobject.tmpl")

	templateBackendPluginRouter, _          = template.ParseFS(templates, "plugin/plugin.tmpl")
	templateBackendPluginResourceHandler, _ = template.ParseFS(templates, "plugin/handler_resource.tmpl")
	templateBackendPluginModelsHandler, _   = template.ParseFS(templates, "plugin/handler_models.tmpl")
	templateBackendMain, _                  = template.ParseFS(templates, "plugin/main.tmpl")

	templateWatcher, _ = template.ParseFS(templates, "app/watcher.tmpl")
	templateApp, _     = template.ParseFS(templates, "app/app.tmpl")

	templateOperatorKubeconfig, _ = template.ParseFS(templates, "operator/kubeconfig.tmpl")
	templateOperatorMain, _       = template.ParseFS(templates, "operator/main.tmpl")
	templateOperatorConfig, _     = template.ParseFS(templates, "operator/config.tmpl")

	templateManifestGoFile, _ = template.New("manifest_go.tmpl").Funcs(funcMap).ParseFS(templates, "manifest_go.tmpl")
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
			return fmt.Sprintf("parsed, _ := time.Parse(time.RFC3339, %s)\nreturn parsed", varName)
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
	Name     string
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
	Package          string
	Group            string
	Version          string
	Kind             string
	Plural           string
	Scope            string
	SelectableFields []SchemaMetadataSelectableField
	FuncPrefix       string
}

type SchemaMetadataSelectableField struct {
	Field    string
	Optional bool
	Type     string
}

func (SchemaMetadata) ToObjectPath(s string) string {
	parts := make([]string, 0)
	if len(s) > 0 && s[0] == '.' {
		s = s[1:]
	}
	for i, part := range strings.Split(s, ".") {
		if i == 0 && part == "metadata" {
			part = "ObjectMeta"
		}
		if len(part) > 0 {
			part = strings.ToUpper(part[:1]) + part[1:]
		} else {
			part = strings.ToUpper(part)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ".")
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
	Repo            string
	APICodegenPath  string
	Resources       []codegen.KindProperties
	PluginID        string
	KindsAreGrouped bool
}

type extendedBackendPluginRouterTemplateMetadata struct {
	BackendPluginRouterTemplateMetadata
	GVToKind map[schema.GroupVersion][]codegen.KindProperties
}

func (BackendPluginRouterTemplateMetadata) ToPackageName(input string) string {
	return ToPackageName(input)
}

func (BackendPluginRouterTemplateMetadata) ToPackageNameVariable(input string) string {
	return strings.ReplaceAll(ToPackageName(input), "_", "")
}

func (BackendPluginRouterTemplateMetadata) GroupToPackageName(input string) string {
	return ToPackageName(strings.Split(input, ".")[0])
}

// WriteBackendPluginRouter executes the Backend Plugin Router template, and writes out the generated go code to out
func WriteBackendPluginRouter(metadata BackendPluginRouterTemplateMetadata, out io.Writer) error {
	return templateBackendPluginRouter.Execute(out, metadata)
}

// BackendPluginHandlerTemplateMetadata is the metadata required by the Backend Plugin Handler template
type BackendPluginHandlerTemplateMetadata struct {
	codegen.KindProperties
	Repo            string
	APICodegenPath  string
	TypeName        string
	IsResource      bool
	Version         string
	KindPackage     string
	KindsAreGrouped bool
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
	md := extendedBackendPluginRouterTemplateMetadata{
		BackendPluginRouterTemplateMetadata: metadata,
		GVToKind:                            make(map[schema.GroupVersion][]codegen.KindProperties),
	}
	for _, k := range md.Resources {
		gv := schema.GroupVersion{Group: k.Group, Version: k.Current}
		l, ok := md.GVToKind[gv]
		if !ok {
			l = make([]codegen.KindProperties, 0)
		}
		l = append(l, k)
		md.GVToKind[gv] = l
	}
	return templateBackendMain.Execute(out, md)
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
	PackageName      string
	Repo             string
	CodegenPath      string
	Version          string
	KindPackage      string
	KindsAreGrouped  bool
	KindPackageAlias string
}

func (WatcherMetadata) ToPackageName(input string) string {
	return ToPackageName(input)
}

func WriteWatcher(metadata WatcherMetadata, out io.Writer) error {
	if metadata.KindPackageAlias == "" {
		metadata.KindPackageAlias = metadata.MachineName
	}
	metadata.KindPackageAlias = ToPackageName(metadata.KindPackageAlias)
	return templateWatcher.Execute(out, metadata)
}

func WriteOperatorKubeConfig(out io.Writer) error {
	return templateOperatorKubeconfig.Execute(out, nil)
}

type OperatorMainMetadata struct {
	PackageName     string
	ProjectName     string
	Repo            string
	CodegenPath     string
	WatcherPackage  string
	KindsAreGrouped bool
	Resources       []codegen.KindProperties
}

type extendedOperatorMainMetadata struct {
	OperatorMainMetadata
	GVToKind map[schema.GroupVersion][]codegen.KindProperties
}

func (OperatorMainMetadata) ToPackageName(input string) string {
	return ToPackageName(input)
}

func (OperatorMainMetadata) ToPackageNameVariable(input string) string {
	return strings.ReplaceAll(ToPackageName(input), "_", "")
}

func (OperatorMainMetadata) GroupToPackageName(input string) string {
	return ToPackageName(strings.Split(input, ".")[0])
}

func WriteOperatorMain(metadata OperatorMainMetadata, out io.Writer) error {
	md := extendedOperatorMainMetadata{
		OperatorMainMetadata: metadata,
		GVToKind:             make(map[schema.GroupVersion][]codegen.KindProperties),
	}
	for _, k := range md.Resources {
		gv := schema.GroupVersion{Group: k.Group, Version: k.Current}
		l, ok := md.GVToKind[gv]
		if !ok {
			l = make([]codegen.KindProperties, 0)
		}
		l = append(l, k)
		md.GVToKind[gv] = l
	}
	return templateOperatorMain.Execute(out, md)
}

func WriteOperatorConfig(out io.Writer) error {
	return templateOperatorConfig.Execute(out, nil)
}

type ManifestGoFileMetadata struct {
	Package              string
	Repo                 string
	CodegenPath          string
	KindsAreGrouped      bool
	ManifestData         app.ManifestData
	CodegenManifestGroup string
}

func (ManifestGoFileMetadata) ToAdmissionOperationName(input app.AdmissionOperation) string {
	switch strings.ToUpper(string(input)) {
	case string(app.AdmissionOperationCreate):
		return "AdmissionOperationCreate"
	case string(app.AdmissionOperationUpdate):
		return "AdmissionOperationUpdate"
	case string(app.AdmissionOperationDelete):
		return "AdmissionOperationDelete"
	case string(app.AdmissionOperationConnect):
		return "AdmissionOperationConnect"
	case string(app.AdmissionOperationAny):
		return "AdmissionOperationAny"
	default:
		return fmt.Sprintf("AdmissionOperation(\"%s\")", input)
	}
}

func (ManifestGoFileMetadata) ToJSONString(input any) string {
	j, _ := json.Marshal(input)
	return string(j)
}

func (ManifestGoFileMetadata) ToJSONBacktickString(input any) string {
	j, _ := json.Marshal(input)
	return "`" + strings.ReplaceAll(string(j), "`", "` + \"`\" + `") + "`"
}

func (ManifestGoFileMetadata) ToPackageName(input string) string {
	return ToPackageName(input)
}

func (ManifestGoFileMetadata) KindToPackageName(input string) string {
	return ToPackageName(strings.ToLower(input))
}

func (ManifestGoFileMetadata) GroupToPackageName(input string) string {
	return ToPackageName(strings.Split(input, ".")[0])
}

func (ManifestGoFileMetadata) GoKindName(kind string) string {
	if len(kind) > 0 {
		return strings.ToUpper(kind[:1]) + kind[1:]
	}
	return strings.ToUpper(kind)
}

func (ManifestGoFileMetadata) ExportedFieldName(name string) string {
	sanitized := regexp.MustCompile("[^A-Za-z0-9_]").ReplaceAllString(name, "")
	if len(sanitized) > 1 {
		return strings.ToUpper(sanitized[:1]) + sanitized[1:]
	}
	return strings.ToUpper(sanitized)
}

func (m ManifestGoFileMetadata) Packages() []string {
	pkgs := make([]string, 0)
	if m.KindsAreGrouped {
		gvs := make(map[string]string)
		for _, v := range m.ManifestData.Versions {
			gvs[fmt.Sprintf("%s/%s", m.GroupToPackageName(m.CodegenManifestGroup), ToPackageName(v.Name))] = ToPackageName(v.Name)
		}
		for pkg, alias := range gvs {
			pkgs = append(pkgs, fmt.Sprintf("%s \"%s\"", alias, filepath.Join(m.Repo, m.CodegenPath, pkg)))
		}
	} else {
		for _, v := range m.ManifestData.Versions {
			for _, k := range v.Kinds {
				pkgs = append(pkgs, fmt.Sprintf("%s%s \"%s\"", m.KindToPackageName(k.Kind), ToPackageName(v.Name), filepath.Join(m.Repo, m.CodegenPath, m.KindToPackageName(k.Kind), ToPackageName(v.Name))))
			}
		}
	}
	// Sort for consistent output
	slices.Sort(pkgs)
	return pkgs
}

func (ManifestGoFileMetadata) StripLeadingSlash(path string) string {
	for len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}
	return path
}

func WriteManifestGoFile(metadata ManifestGoFileMetadata, out io.Writer) error {
	return templateManifestGoFile.Execute(out, metadata)
}

type AppMetadata struct {
	PackageName     string
	ProjectName     string
	Repo            string
	CodegenPath     string
	WatcherPackage  string
	KindsAreGrouped bool
	Resources       []AppMetadataKind
}

type AppMetadataKind struct {
	codegen.KindProperties
	Versions []string
}

type extendedAppMetadata struct {
	AppMetadata
	GVToKindAll     map[schema.GroupVersion][]codegen.KindProperties
	GVToKindCurrent map[schema.GroupVersion][]codegen.KindProperties
}

func (AppMetadata) ToPackageName(input string) string {
	return ToPackageName(input)
}

func (AppMetadata) ToPackageNameVariable(input string) string {
	return strings.ReplaceAll(ToPackageName(input), "_", "")
}

func (AppMetadata) GroupToPackageName(input string) string {
	return ToPackageName(strings.Split(input, ".")[0])
}

func WriteAppGoFile(metadata AppMetadata, out io.Writer) error {
	md := extendedAppMetadata{
		AppMetadata:     metadata,
		GVToKindAll:     make(map[schema.GroupVersion][]codegen.KindProperties),
		GVToKindCurrent: make(map[schema.GroupVersion][]codegen.KindProperties),
	}
	for _, k := range md.Resources {
		gv := schema.GroupVersion{Group: k.Group, Version: k.Current}
		l, ok := md.GVToKindCurrent[gv]
		if !ok {
			l = make([]codegen.KindProperties, 0)
		}
		l = append(l, k.KindProperties)
		md.GVToKindCurrent[gv] = l
		for _, v := range k.Versions {
			gv := schema.GroupVersion{Group: k.Group, Version: v}
			l, ok := md.GVToKindAll[gv]
			if !ok {
				l = make([]codegen.KindProperties, 0)
			}
			l = append(l, k.KindProperties)
			md.GVToKindAll[gv] = l
		}
	}
	return templateApp.Execute(out, md)
}

type ConstantsMetadata struct {
	Package string
	Group   string
	Version string
}

func WriteConstantsFile(metadata ConstantsMetadata, out io.Writer) error {
	return templateConstants.Execute(out, metadata)
}

type GoResourceClientMetadata struct {
	PackageName  string
	KindName     string
	KindPrefix   string
	Subresources []GoResourceClientSubresource
	CustomRoutes []GoResourceClientCustomRoute
}

type GoResourceClientCustomRoute struct {
	TypeName    string
	Path        string
	Method      string
	HasParams   bool
	HasBody     bool
	ParamValues []GoResourceClientParamValues
}

type GoResourceClientParamValues struct {
	Key       string
	FieldName string
}

type GoResourceClientSubresource struct {
	FieldName   string
	Subresource string
}

func WriteGoResourceClient(metadata GoResourceClientMetadata, out io.Writer) error {
	// sort custom route data for consistent output
	slices.SortFunc(metadata.Subresources, func(a, b GoResourceClientSubresource) int {
		return strings.Compare(a.Subresource, b.Subresource)
	})
	slices.SortFunc(metadata.CustomRoutes, func(a, b GoResourceClientCustomRoute) int {
		return strings.Compare(fmt.Sprintf("%s|%s", a.Path, a.Method), fmt.Sprintf("%s|%s", b.Path, b.Method))
	})
	for i := 0; i < len(metadata.CustomRoutes); i++ {
		slices.SortFunc(metadata.CustomRoutes[i].ParamValues, func(a GoResourceClientParamValues, b GoResourceClientParamValues) int {
			return strings.Compare(a.FieldName, b.FieldName)
		})
	}
	return templateGoResourceClient.Execute(out, metadata)
}

type RuntimeObjectWrapperMetadata struct {
	PackageName               string
	WrapperTypeName           string
	TypeName                  string
	HasObjectMeta             bool
	HasListMeta               bool
	AddDeepCopyForTypeName    bool
	KubernetesCodegenComments bool
}

func WriteRuntimeObjectWrapper(metadata RuntimeObjectWrapperMetadata, out io.Writer) error {
	return templateRuntimeObject.Execute(out, metadata)
}

// ToPackageName sanitizes an input into a deterministic allowed go package name.
// It is used to turn kind names or versions into package names when performing go code generation.
func ToPackageName(input string) string {
	return regexp.MustCompile(`([^A-Za-z0-9_])`).ReplaceAllString(input, "_")
}
