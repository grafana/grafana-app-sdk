package codegen

type AppManifest interface {
	Name() string
	Kinds() []Kind
	Properties() AppManifestProperties
}

type AppManifestProperties struct {
	AppName   string `json:"appName"`
	Group     string `json:"group"`
	FullGroup string `json:"fullGroup"`
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
