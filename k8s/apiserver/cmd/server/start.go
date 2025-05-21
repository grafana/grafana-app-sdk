package server

import (
	"context"

	"github.com/spf13/cobra"
	genericapiserver "k8s.io/apiserver/pkg/server"

	"github.com/grafana/grafana-app-sdk/k8s/apiserver"
)

func NewCommandStartServer(ctx context.Context, installers []apiserver.APIServerInstaller) *cobra.Command {
	o := apiserver.NewOptions(installers)
	cmd := &cobra.Command{
		Short: "Launch a API server",
		Long:  "Launch a API server",
		RunE: func(c *cobra.Command, args []string) error {
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
