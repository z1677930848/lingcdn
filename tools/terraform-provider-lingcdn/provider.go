package main

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

type lingcdnProvider struct {
	version string
}

func NewProvider() func() provider.Provider {
	return func() provider.Provider {
		return &lingcdnProvider{version: "0.1.0"}
	}
}

func (p *lingcdnProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "lingcdn"
	resp.Version = p.version
}

func (p *lingcdnProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Interact with LingCDN control plane API.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Description: "Control plane base URL, e.g. https://cdn.example.com",
				Required:    true,
			},
			"api_token": schema.StringAttribute{
				Description: "Admin API token",
				Required:    true,
				Sensitive:   true,
			},
		},
	}
}

type providerConfig struct {
	endpoint string
	token    string
}

func (p *lingcdnProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg struct {
		Endpoint *string `tfsdk:"endpoint"`
		APIToken *string `tfsdk:"api_token"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.Endpoint == nil || cfg.APIToken == nil {
		resp.Diagnostics.AddError("Missing configuration", "endpoint and api_token are required")
		return
	}
	resp.ResourceData = &providerConfig{endpoint: *cfg.Endpoint, token: *cfg.APIToken}
	resp.DataSourceData = resp.ResourceData
}

func (p *lingcdnProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		newDomainResource,
	}
}

func (p *lingcdnProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
