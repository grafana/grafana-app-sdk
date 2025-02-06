package codegen

type AppManifest interface {
	Name() string
	Kinds() []Kind
	Properties() AppManifestProperties
}

type AppManifestProperties struct {
	AppName          string                                `json:"appName"`
	Group            string                                `json:"group"`
	FullGroup        string                                `json:"fullGroup"`
	ExtraPermissions AppManifestPropertiesExtraPermissions `json:"extraPermissions"`
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
	Props    AppManifestProperties
	AllKinds []Kind
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
