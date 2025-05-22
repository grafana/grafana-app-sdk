package codegen

type AppManifest interface {
	Name() string
	Kinds() []Kind
	Properties() AppManifestProperties
	CustomRoutes() []AppManifestCustomRoute
}

type AppManifestCustomRoute struct {
	Group      string                            `json:"group"`
	Version    string                            `json:"version"`
	Namespaced map[string]map[string]CustomRoute `json:"namespaced"`
	Root       map[string]map[string]CustomRoute `json:"root"`
}

type AppManifestProperties struct {
	AppName          string                                `json:"appName"`
	Group            string                                `json:"group"`
	FullGroup        string                                `json:"fullGroup"`
	ExtraPermissions AppManifestPropertiesExtraPermissions `json:"extraPermissions"`
	OperatorURL      *string                               `json:"operatorURL,omitempty"`
}

type AppManifestPropertiesExtraPermissions struct {
	AccessKinds []AppManifestKindPermission `json:"accessKinds,omitempty"`
}

type AppManifestKindPermission struct {
	Group    string   `json:"group"`
	Resource string   `json:"resource"`
	Actions  []string `json:"actions"`
}

type SimpleManifest struct {
	Props                AppManifestProperties
	AllKinds             []Kind
	ManifestCustomRoutes []AppManifestCustomRoute
}

func (m *SimpleManifest) Name() string {
	return m.Props.AppName
}

func (m *SimpleManifest) Properties() AppManifestProperties {
	return m.Props
}

func (m *SimpleManifest) Kinds() []Kind {
	return m.AllKinds
}

func (m *SimpleManifest) CustomRoutes() []AppManifestCustomRoute {
	return m.ManifestCustomRoutes
}
