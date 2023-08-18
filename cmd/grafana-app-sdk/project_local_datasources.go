package main

// Datasource configuration. New datasources for the local dev environment codegen should go here
var (
	localDatasourceConfigs = map[string]dataSourceConfig{
		"cortex": {
			Access: "proxy",
			Name:   "grafana-k3d-cortex-prom",
			Type:   "prometheus",
			UID:    "grafana-prom-cortex",
			URL:    "http://cortex.default.svc.cluster.local:9009/api/prom",
		},
		"tempo": {
			Access: "proxy",
			Name:   "grafana-k3d-tempo",
			Type:   "tempo",
			UID:    "grafana-traces-tempo",
			URL:    "http://tempo.default.svc.cluster.local:3100",
		},
	}

	localDatasourceFiles = map[string][]string{
		"cortex": {"templates/local/generated/datasources/cortex.yaml"},
		"tempo":  {"templates/local/generated/datasources/tempo.yaml"},
	}
)
