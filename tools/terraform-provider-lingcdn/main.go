// Terraform provider skeleton for LingCDN control plane.
// Build: go build -o terraform-provider-lingcdn
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

const providerAddr = "registry.terraform.io/lingcdn/lingcdn"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with debug logging")
	flag.Parse()

	opts := providerserver.ServeOpts{
		Address: providerAddr,
		Debug:   debug,
	}

	err := providerserver.Serve(context.Background(), NewProvider, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "provider error: %s\n", err.Error())
		os.Exit(1)
	}
}
