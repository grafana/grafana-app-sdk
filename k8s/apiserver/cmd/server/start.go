package server

import (
	"context"

	"github.com/spf13/cobra"
	genericapiserver "k8s.io/apiserver/pkg/server"

	"github.com/grafana/grafana-app-sdk/k8s/apiserver" //nolint:staticcheck // This deprecated runner depends on apiserver until both are removed.
)

// Deprecated: The standalone server runner will be removed in the next minor
// release of grafana-app-sdk. It was not moved to
// github.com/grafana/grafana/pkg/apiserver/appsdk.
// See the k8s/apiserver README for migration details.
func NewCommandStartServer(ctx context.Context, o *apiserver.Options) *cobra.Command {
	cmd := &cobra.Command{
		Short: "Launch a API server",
		Long:  "Launch a API server",
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := o.Validate(); err != nil {
				return err
			}

			cfg, err := o.Config()
			if err != nil {
				return err
			}
			server, err := cfg.NewServer(genericapiserver.NewEmptyDelegate())
			if err != nil {
				return err
			}
			prepared := server.PrepareRun()
			return prepared.RunWithContext(ctx)
		},
	}
	cmd.SetContext(ctx)

	flags := cmd.Flags()
	o.AddFlags(flags)
	return cmd
}
