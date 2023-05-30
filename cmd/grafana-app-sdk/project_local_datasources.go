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
	}

	localDatasourceFiles = map[string][]string{
		"cortex": {"templates/local/generated/datasources/cortex.yaml"},
	}
)
