package apiserver

import (
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericoptions "k8s.io/apiserver/pkg/server/options"
)

type Options struct {
	RecommendedOptions *genericoptions.RecommendedOptions
	scheme             *runtime.Scheme
	codecs             serializer.CodecFactory
	installers         []APIServerInstaller
}

func NewOptions(installers []APIServerInstaller) *Options {
	scheme := newScheme()
	codecs := serializer.NewCodecFactory(scheme)

	return &Options{
		scheme: scheme,
		codecs: codecs,
	}
}

func (o *Options) AddFlags(fs *pflag.FlagSet) {
	o.RecommendedOptions.AddFlags(fs)
}

func (o *Options) Validate() []error {
	errs := []error{}
	errs = append(errs, o.RecommendedOptions.Validate()...)
	return errs
}

func (o *Options) ApplyTo(cfg *Config) error {
	for _, installer := range o.installers {
		pluginName, plugin := installer.AdmissionPlugin()
		if pluginName != "" {
			o.RecommendedOptions.Admission.Plugins.Register(pluginName, plugin)
			o.RecommendedOptions.Admission.RecommendedPluginOrder = append(o.RecommendedOptions.Admission.RecommendedPluginOrder, pluginName)
			o.RecommendedOptions.Admission.EnablePlugins = append(o.RecommendedOptions.Admission.EnablePlugins, pluginName)
		}
	}
	if err := o.RecommendedOptions.ApplyTo(cfg.Generic); err != nil {
		return err
	}

	cfg.UpdateOpenAPIConfig()

	return nil
}

func (o *Options) Config() (*Config, error) {
	cfg := &Config{
		Generic:    genericapiserver.NewRecommendedConfig(o.codecs),
		scheme:     o.scheme,
		codecs:     o.codecs,
		installers: o.installers,
	}

	if err := o.ApplyTo(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
