package metrics

import "github.com/prometheus/client_golang/prometheus"

// ExporterConfig is the configuration used for the Exporter
type ExporterConfig struct {
	Registerer prometheus.Registerer
	Gatherer   prometheus.Gatherer
	Port       int
}

// Config is the general set of configuration options for creating prometheus Collectors
type Config struct {
	Namespace string
}
