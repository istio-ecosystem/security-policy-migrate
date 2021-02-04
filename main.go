package main

import (
	"log"
	"os"

	"github.com/spf13/cobra"
)

var (
	kubeconfig    string
	configContext string
	ignoreError   bool
	perNamespace  string
	version       string
)

func main() {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.Ldate | log.Ltime)
	cmd := rootCmd(os.Args[1:])
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd(args []string) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "convert",
		Short:             "Convert Istio v1alpha1 authentication policy to v1beta1 version (PeerAuthentication, RequestAuthentication, AuthorizationPolicy).",
		SilenceUsage:      true,
		DisableAutoGenTag: true,
		Example: `
# Convert the v1alpha1 authentication policy in the current cluster and output the beta policy to beta-policies.yaml:
./convert > beta-policy.yaml
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if kubeconfig != "" {
				log.Printf("configured kubeconfig: %s", kubeconfig)
			}
			if configContext != "" {
				log.Printf("configured context: %s", configContext)
			}
			client, err := newKubeClient(kubeconfig, configContext)
			if err != nil {
				log.Fatalf("failed to create kube client: %v", err)
			}
			return client.convert()
		},
		Version: version,
	}
	cmd.SetArgs(args)
	cmd.PersistentFlags().StringVarP(&kubeconfig, "kubeconfig", "c", "",
		"kubernetes configuration file")
	cmd.PersistentFlags().StringVar(&configContext, "context", "",
		"the name of the kubeconfig context to use")
	cmd.PersistentFlags().BoolVar(&ignoreError, "ignore-error", false, "ignore any errors found in "+
		"the conversion and still generate the converted beta policies, use with caution as the converted policies may not work as expected")
	cmd.PersistentFlags().StringVarP(&perNamespace, "per-namespace", "", "", "store policies per-namespace "+
		"so that you could verify and apply the generated policies incrementally in separate yaml file per-namespace")
	return cmd
}
