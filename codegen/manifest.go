package codegen

type AppManifest interface {
	Name() string
	Kinds() []Kind
	Properties() AppManifestProperties
}

type AppManifestProperties struct {
	AppName string
	Group   string
}
