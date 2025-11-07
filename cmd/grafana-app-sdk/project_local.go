package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"embed"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/grafana/codejen"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/cuekind"
)

//go:embed templates/local/* templates/local/scripts/* templates/local/generated/datasources/*
var localEnvFiles embed.FS

// localEnvConfig is the configuration object used for the generation of local dev env resources
type localEnvConfig struct {
	Port                      int                       `json:"port" yaml:"port"`
	KubePort                  int                       `json:"kubePort" yaml:"kubePort"`
	Datasources               []string                  `json:"datasources" yaml:"datasources"`
	DatasourceConfigs         []dataSourceConfig        `json:"datasourceConfigs" yaml:"datasourceConfigs"`
	PluginJSON                map[string]any            `json:"pluginJson" yaml:"pluginJson"`
	PluginSecureJSON          map[string]any            `json:"pluginSecureJson" yaml:"pluginSecureJson"`
	OperatorImage             string                    `json:"operatorImage" yaml:"operatorImage"`
	Webhooks                  localEnvWebhookConfig     `json:"webhooks" yaml:"webhooks"`
	GenerateGrafanaDeployment bool                      `json:"generateGrafanaDeployment" yaml:"generateGrafanaDeployment"`
	GrafanaImage              string                    `json:"grafanaImage" yaml:"grafanaImage"`
	GrafanaInstallPlugins     string                    `json:"grafanaInstallPlugins" yaml:"grafanaInstallPlugins"`
	GrafanaWithAnonymousAuth  bool                      `json:"grafanaWithAnonymousAuth" yaml:"grafanaWithAnonymousAuth"`
	AdditionalVolumeMounts    []additionalMountedVolume `json:"additionalVolumeMounts" yaml:"additionalVolumeMounts"`
}

type dataSourceConfig struct {
	Access       string   `json:"access" yaml:"access"`
	Editable     bool     `json:"editable" yaml:"editable"`
	IsDefault    bool     `json:"isDefault" yaml:"isDefault"`
	Name         string   `json:"name" yaml:"name"`
	Type         string   `json:"type" yaml:"type"`
	UID          string   `json:"uid" yaml:"uid"`
	URL          string   `json:"url" yaml:"url"`
	Dependencies []string `json:"dependencies" yaml:"dependencies"`
}

type localEnvWebhookConfig struct {
	Mutating   bool `json:"mutating" yaml:"mutating"`
	Validating bool `json:"validating" yaml:"validating"`
	Converting bool `json:"converting" yaml:"converting"`
	Port       int  `json:"port" yaml:"port"`
}

type additionalMountedVolume struct {
	SourcePath string `json:"sourcePath" yaml:"sourcePath"`
	MountPath  string `json:"mountPath" yaml:"mountPath"`
}

func projectLocalEnvInit(cmd *cobra.Command, _ []string) error {
	// Path (optional)
	path, err := cmd.Flags().GetString("path")
	if err != nil {
		return err
	}
	modName, err := getGoModule(filepath.Join(path, "go.mod"))
	if err != nil {
		return err
	}
	return initializeLocalEnvFiles(path, modName, modName)
}

func initializeLocalEnvFiles(basePath, clusterName, operatorImageName string) error {
	localPath := filepath.Join(basePath, "local")

	// Write the default local config file
	cfgTemplate, err := template.ParseFS(localEnvFiles, "templates/local/config.yaml")
	if err != nil {
		return err
	}
	cfgBytes := bytes.Buffer{}
	err = cfgTemplate.Execute(&cfgBytes, map[string]string{
		"OperatorImage": operatorImageName,
	})
	if err != nil {
		return err
	}
	err = writeFile(filepath.Join(localPath, "config.yaml"), cfgBytes.Bytes())
	if err != nil {
		return err
	}

	// Write out all scripts
	scripts, err := localEnvFiles.ReadDir("templates/local/scripts")
	if err != nil {
		return err
	}
	for _, script := range scripts {
		scriptTemplate, err := template.ParseFS(localEnvFiles, filepath.Join("templates", "local", "scripts", script.Name()))
		if err != nil {
			return err
		}
		buf := bytes.Buffer{}
		err = scriptTemplate.Execute(&buf, map[string]string{
			"ClusterName": clusterName,
		})
		if err != nil {
			return err
		}
		err = writeExecutableFile(filepath.Join(localPath, "scripts", script.Name()), buf.Bytes())
		if err != nil {
			return err
		}
	}

	tiltfile, err := generateTiltfile()
	if err != nil {
		return err
	}
	err = writeFileWithOverwriteConfirm(filepath.Join(localPath, "Tiltfile"), tiltfile)
	if err != nil {
		return err
	}

	err = checkAndMakePath(filepath.Join(localPath, "additional"))
	if err != nil {
		return err
	}
	err = checkAndMakePath(filepath.Join(localPath, "mounted-files", "plugin"))
	if err != nil {
		return err
	}

	return nil
}

// nolint:funlen
func projectLocalEnvGenerate(cmd *cobra.Command, _ []string) error {
	// Path (optional)
	path, err := cmd.Flags().GetString("path")
	if err != nil {
		return err
	}
	sourcePath, err := cmd.Flags().GetString(sourceFlag)
	if err != nil {
		return err
	}
	format, err := cmd.Flags().GetString(formatFlag)
	if err != nil {
		return err
	}
	selector, err := cmd.Flags().GetString(selectorFlag)
	if err != nil {
		return err
	}
	genOperatorState, err := cmd.Flags().GetBool(genOperatorStateFlag)
	if err != nil {
		return err
	}
	useOldManifestKinds, err := cmd.Flags().GetBool("useoldmanifestkinds")
	if err != nil {
		return err
	}
	localPath := filepath.Join(path, "local")
	localGenPath := filepath.Join(localPath, "generated")
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	// Get config
	config, err := getLocalEnvConfig(localPath)
	if err != nil {
		return err
	}

	// Get disablePluginMount ID
	var disablePluginMount bool
	pluginID, err := getPluginID(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		pluginID = "placeholder-plugin-id"
		disablePluginMount = true
	case err != nil:
		return err
	case pluginID == "":
		return errors.New("plugin ID cannot be empty")
	default:
		disablePluginMount = false
	}

	// Generate the k3d config (this has to be generated, as it needs to mount an absolute path on the host)
	k3dConfig, err := generateK3dConfig(absPath, *config)
	if err != nil {
		return err
	}
	err = writeFile(filepath.Join(localGenPath, "k3d-config.json"), k3dConfig)
	if err != nil {
		return err
	}

	err = updateLocalConfigFromManifest(config, format, sourcePath, useOldManifestKinds, selector)
	if err != nil {
		return err
	}

	// Generate the k8s YAML bundle
	parseFunc := func() (codejen.Files, error) {
		switch format {
		case FormatCUE:
			parser, err := cuekind.NewParser()
			if err != nil {
				return nil, err
			}
			generator, err := codegen.NewGenerator(parser.KindParser(cuekind.ParseConfig{
				GenOperatorState: genOperatorState,
				UseOldKinds:      useOldManifestKinds,
			}), os.DirFS(sourcePath))
			if err != nil {
				return nil, err
			}
			return generator.Generate(cuekind.CRDGenerator(yaml.Marshal, "yaml"), selector)
		case FormatNone:
			return codejen.Files{}, nil
		default:
			return nil, fmt.Errorf("unknown kind format '%s'", format)
		}
	}

	k8sYAML, genProps, err := generateKubernetesYAML(parseFunc, pluginID, disablePluginMount, *config)
	if err != nil {
		return err
	}
	err = writeFile(filepath.Join(localGenPath, "dev-bundle.yaml"), k8sYAML)
	if err != nil {
		return err
	}
	aggregateScript, err := generateAggregationScript(*config, genProps)
	if err != nil {
		return err
	}
	err = writeExecutableFile(filepath.Join(localGenPath, "aggregate-apiserver.sh"), aggregateScript)
	if err != nil {
		return err
	}

	return nil
}

func getLocalEnvConfig(localPath string) (*localEnvConfig, error) {
	// Read config (try YAML first, then JSON)
	config := localEnvConfig{
		GenerateGrafanaDeployment: true,
		GrafanaImage:              "grafana/grafana-enterprise:11.2.2",
	}
	if _, err := os.Stat(filepath.Join(localPath, "config.yaml")); err == nil {
		cfgBytes, err := os.ReadFile(filepath.Join(localPath, "config.yaml"))
		if err != nil {
			return nil, err
		}
		err = yaml.Unmarshal(cfgBytes, &config)
		if err != nil {
			return nil, err
		}
	} else if _, err = os.Stat(filepath.Join(localPath, "config.json")); err == nil {
		cfgBytes, err := os.ReadFile(filepath.Join(localPath, "config.json"))
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(cfgBytes, &config)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("nether %s/config.yaml nor %s/config.json not found, please run `grafana-app-sdk project local init` to generate", localPath, localPath)
	}
	return &config, nil
}

func getPluginID(rootPath string) (string, error) {
	pluginJSONPath := filepath.Join(rootPath, "plugin", "src", "plugin.json")
	if _, err := os.Stat(pluginJSONPath); err != nil {
		return "", err
	}
	pluginJSONFile, err := os.ReadFile(pluginJSONPath)
	if err != nil {
		return "", err
	}

	type pluginJSON struct {
		ID string `json:"id"`
	}
	um := pluginJSON{}
	err = json.Unmarshal(pluginJSONFile, &um)
	return um.ID, err
}

func generateK3dConfig(projectRoot string, config localEnvConfig) ([]byte, error) {
	k3dConfigTmpl, err := template.ParseFS(localEnvFiles, "templates/local/generated/k3d-config.json")
	if err != nil {
		return nil, err
	}
	additionalVolumes := make([]additionalMountedVolume, 0)
	for _, v := range config.AdditionalVolumeMounts {
		if len(v.SourcePath) > 1 && v.SourcePath[0] != '/' {
			if v.SourcePath[0:2] == "./" {
				v.SourcePath = v.SourcePath[2:]
			}
			v.SourcePath = filepath.Join(projectRoot, v.SourcePath)
		}
		additionalVolumes = append(additionalVolumes, v)
	}
	buf := &bytes.Buffer{}
	err = k3dConfigTmpl.Execute(buf, map[string]any{
		"ProjectRoot":       projectRoot,
		"BindPort":          strconv.Itoa(config.Port),
		"AdditionalVolumes": additionalVolumes,
	})
	return buf.Bytes(), err
}

type scriptGenProperties struct {
	Port int
	CRDs []yamlGenPropsCRD
}

type yamlGenProperties struct {
	PluginID                  string
	PluginIDKube              string
	DisableGrafanaPluginMount bool
	CRDs                      []yamlGenPropsCRD
	Services                  []yamlGenPropsService
	JSONData                  map[string]string
	SecureJSONData            map[string]string
	Datasources               []dataSourceConfig
	OperatorImage             string
	WebhookProperties         yamlGenPropsWebhooks
	GenerateGrafanaDeployment bool
	GrafanaImage              string
	GrafanaInstallPlugins     string
	GrafanaAnonymousAuth      string
	APIGroups                 map[string][]string // map of group -> list of supported versions
}

type yamlGenPropsCRD struct {
	MachineName       string
	PluralMachineName string
	Group             string
	Versions          []string
}

type yamlGenPropsService struct {
	KubeName string
}

type yamlGenPropsWebhooks struct {
	Enabled    bool
	Port       int
	Mutating   string
	Validating string
	Converting string
	Base64Cert string
	Base64Key  string
	Base64CA   string
}

type crdYAML struct {
	Spec struct {
		Group string `yaml:"group"`
		Names struct {
			Kind   string `yaml:"kind"`
			Plural string `yaml:"plural"`
		} `yaml:"names"`
		Versions []struct {
			Name   string `yaml:"name"`
			Served bool   `yaml:"served"`
		} `yaml:"versions"`
	} `yaml:"spec"`
}

var kubeReplaceRegexp = regexp.MustCompile(`[^a-z0-9\-]`)

//nolint:funlen,errcheck,revive,gocyclo,gocognit
func generateKubernetesYAML(crdGenFunc func() (codejen.Files, error), pluginID string, disablePluginMount bool, config localEnvConfig) ([]byte, yamlGenProperties, error) {
	output := bytes.Buffer{}
	props := yamlGenProperties{
		PluginID:                  pluginID,
		PluginIDKube:              kubeReplaceRegexp.ReplaceAllString(strings.ToLower(pluginID), "-"),
		DisableGrafanaPluginMount: disablePluginMount,
		CRDs:                      make([]yamlGenPropsCRD, 0),
		Services:                  make([]yamlGenPropsService, 0),
		Datasources:               make([]dataSourceConfig, 0),
		JSONData:                  make(map[string]string),
		SecureJSONData:            make(map[string]string),
		OperatorImage:             config.OperatorImage,
		WebhookProperties: yamlGenPropsWebhooks{
			Enabled: config.Webhooks.Mutating || config.Webhooks.Validating || config.Webhooks.Converting,
		},
		GenerateGrafanaDeployment: config.GenerateGrafanaDeployment,
		GrafanaImage:              config.GrafanaImage,
		GrafanaInstallPlugins:     config.GrafanaInstallPlugins,
		APIGroups:                 make(map[string][]string),
	}
	props.Services = append(props.Services, yamlGenPropsService{
		KubeName: "grafana",
	})
	if props.OperatorImage != "" {
		props.Services = append(props.Services, yamlGenPropsService{
			KubeName: "operator",
		})
	}
	if props.OperatorImage != "" {
		// Prefix with "localhost/" to ensure that our local build uses our locally-built image
		props.OperatorImage = fmt.Sprintf("localhost/%s", props.OperatorImage)
	}
	if config.GrafanaWithAnonymousAuth {
		props.GrafanaAnonymousAuth = "Viewer"
	}

	if props.WebhookProperties.Enabled {
		if config.Webhooks.Port > 0 {
			props.WebhookProperties.Port = config.Webhooks.Port
		} else {
			props.WebhookProperties.Port = 8443
		}
		if config.Webhooks.Mutating {
			props.WebhookProperties.Mutating = "/mutate"
		}
		if config.Webhooks.Validating {
			props.WebhookProperties.Validating = "/validate"
		}
		if config.Webhooks.Converting {
			props.WebhookProperties.Converting = "/convert"
		}
		// Generate cert bundle
		bundle, err := generateCerts(fmt.Sprintf("%s-operator.default.svc", props.PluginID))
		if err != nil {
			return nil, props, err
		}
		props.WebhookProperties.Base64Cert = base64.StdEncoding.EncodeToString(bundle.cert)
		props.WebhookProperties.Base64Key = base64.StdEncoding.EncodeToString(bundle.key)
		props.WebhookProperties.Base64CA = base64.StdEncoding.EncodeToString(bundle.ca)
	}

	// Generate CRD YAML files, add the CRD metadata to the props
	crdFiles, err := crdGenFunc()
	if err != nil {
		return nil, props, err
	}
	for _, f := range crdFiles {
		// If converting webhooks are enabled, upate the yaml
		// TODO: this is a hack workaround for now, this should eventually be in the CRD generator
		if props.WebhookProperties.Converting != "" {
			rawCRD := make(map[string]any)
			if err := yaml.Unmarshal(f.Data, &rawCRD); err != nil {
				return nil, props, err
			}
			spec, ok := rawCRD["spec"].(map[string]any)
			if !ok {
				return nil, props, errors.New("could not parse CRD")
			}
			spec["conversion"] = map[string]any{
				"strategy": "Webhook",
				"webhook": map[string]any{
					"conversionReviewVersions": []string{"v1"},
					"clientConfig": map[string]any{
						"service": map[string]any{
							"name":      props.PluginID + "-operator",
							"namespace": "default",
							"path":      "/convert",
						},
						"caBundle": props.WebhookProperties.Base64CA,
					},
				},
			}
			rawCRD["spec"] = spec
			f.Data, err = yaml.Marshal(rawCRD)
			if err != nil {
				return nil, props, fmt.Errorf("unable to re-marshal CRD YAML after added conversion strategy: %w", err)
			}
		}

		output.Write(append(f.Data, []byte("\n---\n")...))
		yml := crdYAML{}
		err = yaml.Unmarshal(f.Data, &yml)
		if err != nil {
			return nil, props, err
		}
		versions := make([]string, 0)
		groupVersions := make([]string, 0)
		if v, ok := props.APIGroups[yml.Spec.Group]; ok {
			groupVersions = v
		}
		for _, v := range yml.Spec.Versions {
			if v.Served {
				versions = append(versions, v.Name)
				if !slices.Contains(groupVersions, v.Name) {
					groupVersions = append(groupVersions, v.Name)
				}
			}
		}
		props.CRDs = append(props.CRDs, yamlGenPropsCRD{
			MachineName:       strings.ToLower(yml.Spec.Names.Kind),
			PluralMachineName: strings.ToLower(yml.Spec.Names.Plural),
			Group:             yml.Spec.Group,
			Versions:          versions,
		})
		props.APIGroups[yml.Spec.Group] = groupVersions
	}

	// RBAC for CRDs
	tmplRoles, err := template.ParseFS(localEnvFiles, "templates/local/generated/crd_roles.yaml")
	if err != nil {
		return nil, props, err
	}
	for _, c := range props.CRDs {
		err = tmplRoles.Execute(&output, c)
		if err != nil {
			return nil, props, err
		}
		output.Write([]byte("\n---\n"))
	}

	// RBAC for aggregator
	tmplAggregatorAccess, err := template.ParseFS(localEnvFiles, "templates/local/generated/aggregator-access.yaml")
	if err != nil {
		return nil, props, err
	}
	err = tmplAggregatorAccess.Execute(&output, props)
	if err != nil {
		return nil, props, err
	}
	output.Write([]byte("\n---\n"))

	// Datasources
	addedDeps := make(map[string]struct{})
	for i, ds := range config.Datasources {
		err := localGenerateDatasourceYAML(ds, i == 0, &props, addedDeps, &output)
		if err != nil {
			return nil, props, err
		}
		output.WriteString("\n---\n")
	}
	if len(config.DatasourceConfigs) > 0 {
		props.Datasources = append(props.Datasources, config.DatasourceConfigs...)
	}

	// Grafana deployment
	err = localGenerateGrafanaYAML(config, &props, &output)

	// Operator deployment
	if config.OperatorImage != "" {
		output.WriteString("---\n")
		tmplOperator, err := template.ParseFS(localEnvFiles, "templates/local/generated/operator.yaml")
		if err != nil {
			return nil, props, err
		}
		err = tmplOperator.Execute(&output, props)
		if err != nil {
			return nil, props, err
		}
	}
	return output.Bytes(), props, err
}

func generateAggregationScript(config localEnvConfig, genProps yamlGenProperties) ([]byte, error) {
	tmpl, err := template.ParseFS(localEnvFiles, "templates/local/generated/configure-grafana.sh")
	if err != nil {
		return nil, err
	}
	output := bytes.Buffer{}
	err = tmpl.Execute(&output, scriptGenProperties{
		Port: config.Port,
		CRDs: genProps.CRDs,
	})
	if err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

//nolint:revive
func localGenerateDatasourceYAML(datasource string, isDefault bool, props *yamlGenProperties, depsMap map[string]struct{}, out io.Writer) error {
	datasource = strings.ToLower(datasource)
	cfg, ok := localDatasourceConfigs[datasource]
	if !ok {
		return fmt.Errorf("unsupported datasource '%s'", datasource)
	}
	files := make([]string, 0)
	for _, dep := range cfg.Dependencies {
		if _, ok := depsMap[dep]; ok {
			continue
		}
		files = append(files, localDatasourceDependencyManifests[dep]...)
		depsMap[dep] = struct{}{}
	}
	dsFiles, ok := localDatasourceFiles[datasource]
	files = append(files, dsFiles...)
	if ok {
		for i, f := range files {
			if i > 0 {
				_, err := out.Write([]byte("\n---\n"))
				if err != nil {
					return err
				}
			}
			tmplDatasourceFile, err := template.ParseFS(localEnvFiles, f)
			if err != nil {
				return err
			}
			err = tmplDatasourceFile.Execute(out, props)
			if err != nil {
				return err
			}
		}
	}
	if isDefault {
		cfg.IsDefault = true
	}
	props.Datasources = append(props.Datasources, cfg)
	return nil
}

func localGenerateGrafanaYAML(config localEnvConfig, props *yamlGenProperties, out io.Writer) error {
	for k, v := range config.PluginJSON {
		val, err := parsePluginJSONValue(v)
		if err != nil {
			return fmt.Errorf("unable to parse pluginJson key '%s'", k)
		}
		props.JSONData[k] = val
	}
	config.PluginSecureJSON["kubeconfig"] = "cluster"
	config.PluginSecureJSON["kubenamespace"] = "default"
	for k, v := range config.PluginSecureJSON {
		val, err := parsePluginJSONValue(v)
		if err != nil {
			return fmt.Errorf("unable to parse pluginSecureJson key '%s'", k)
		}
		props.SecureJSONData[k] = val
	}

	tmplGrafana, err := template.ParseFS(localEnvFiles, "templates/local/generated/grafana.yaml")
	if err != nil {
		return err
	}
	err = tmplGrafana.Execute(out, props)
	if err != nil {
		return err
	}
	return nil
}

func parsePluginJSONValue(v any) (string, error) {
	switch cast := v.(type) {
	case map[string]any, []any:
		val, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(val), nil
	case string:
		return cast, nil
	case int, int32, int64:
		return strconv.Itoa(v.(int)), nil
	case float32:
		return strconv.FormatFloat(float64(cast), 'E', -1, 32), nil
	case float64:
		return strconv.FormatFloat(cast, 'E', -1, 64), nil
	case bool:
		return strconv.FormatBool(cast), nil
	default:
		return "", errors.New("unknown type")
	}
}

func generateTiltfile() ([]byte, error) {
	buf := bytes.Buffer{}
	tmplGrafana, err := template.ParseFS(localEnvFiles, "templates/local/Tiltfile")
	if err != nil {
		return nil, err
	}
	err = tmplGrafana.Execute(&buf, struct{}{})
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), err
}

var ca = &x509.Certificate{
	SerialNumber: big.NewInt(2019),
	Subject: pkix.Name{
		Organization:  []string{"Grafana-App-SDK Generated Local Environment CA"},
		Country:       []string{"US"},
		Province:      []string{""},
		Locality:      []string{"San Francisco"},
		StreetAddress: []string{"Golden Gate Bridge"},
		PostalCode:    []string{"94016"},
	},
	NotBefore:             time.Now(),
	NotAfter:              time.Now().AddDate(10, 0, 0),
	IsCA:                  true,
	ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
	KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
	BasicConstraintsValid: true,
}

func serverCert(dnsNames []string) *x509.Certificate {
	return &x509.Certificate{
		SerialNumber: big.NewInt(1658),
		Subject: pkix.Name{
			Organization:  []string{"Grafana-App-SDK Generated Local Environment Webhook Server Cert"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"San Francisco"},
			StreetAddress: []string{"Golden Gate Bridge"},
			PostalCode:    []string{"94016"},
		},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		DNSNames:     dnsNames,
	}
}

type certBundle struct {
	cert []byte
	key  []byte
	ca   []byte
}

func generateCerts(dnsName string) (*certBundle, error) {
	caPrivateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, err
	}

	caCertBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivateKey.PublicKey, caPrivateKey)
	if err != nil {
		return nil, err
	}

	caPEM := new(bytes.Buffer)
	err = pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caCertBytes,
	})
	if err != nil {
		return nil, err
	}

	caPrivKeyPEM := new(bytes.Buffer)
	err = pem.Encode(caPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(caPrivateKey),
	})
	if err != nil {
		return nil, err
	}

	certPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, err
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, serverCert([]string{dnsName}), ca, &certPrivKey.PublicKey, caPrivateKey)
	if err != nil {
		return nil, err
	}

	certPEM := new(bytes.Buffer)
	err = pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})
	if err != nil {
		return nil, err
	}

	certPrivKeyPEM := new(bytes.Buffer)
	err = pem.Encode(certPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(certPrivKey),
	})
	if err != nil {
		return nil, err
	}

	return &certBundle{
		cert: certPEM.Bytes(),
		key:  certPrivKeyPEM.Bytes(),
		ca:   caPEM.Bytes(),
	}, nil
}

func updateLocalConfigFromManifest(config *localEnvConfig, format string, cuePath string, useOldManifestKinds bool, selectors ...string) error {
	type manifest struct {
		Kind string           `json:"kind"`
		Spec app.ManifestData `json:"spec"`
	}
	if format == FormatCUE {
		parser, err := cuekind.NewParser()
		if err != nil {
			return err
		}
		generator, err := codegen.NewGenerator[codegen.AppManifest](parser.ManifestParser(cuekind.ParseConfig{
			UseOldKinds: useOldManifestKinds,
		}), os.DirFS(cuePath))
		if err != nil {
			return err
		}
		fs, err := generator.Generate(cuekind.ManifestGenerator(json.Marshal, "json", false, true), selectors...)
		if err != nil {
			return err
		}
		for _, f := range fs {
			md := manifest{}
			err = json.Unmarshal(f.Data, &md)
			if err != nil {
				return err
			}
			if md.Kind != "AppManifest" {
				continue
			}
			for _, v := range md.Spec.Versions {
				for _, k := range v.Kinds {
					if k.Conversion {
						config.Webhooks.Converting = true
					}
					if k.Admission != nil && k.Admission.SupportsAnyValidation() {
						config.Webhooks.Validating = true
					}
					if k.Admission != nil && k.Admission.SupportsAnyMutation() {
						config.Webhooks.Mutating = true
					}
				}
			}
		}
	}
	return nil
}
