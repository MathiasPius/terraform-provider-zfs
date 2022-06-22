package provider

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/appleboy/easyssh-proxy"
)

func init() {
	// Set descriptions to support markdown syntax, this will be used in document generation
	// and the language server.
	schema.DescriptionKind = schema.StringMarkdown

	// Customize the content of descriptions when output. For example you can add defaults on
	// to the exported descriptions if present.
	// schema.SchemaDescriptionBuilder = func(s *schema.Schema) string {
	// 	desc := s.Description
	// 	if s.Default != nil {
	// 		desc += fmt.Sprintf(" Defaults to `%v`.", s.Default)
	// 	}
	// 	return strings.TrimSpace(desc)
	// }
}

type Config struct {
	command_prefix string
	ssh            *easyssh.MakeConfig
}

func New(version string) func() *schema.Provider {
	return func() *schema.Provider {
		p := &schema.Provider{
			Schema: map[string]*schema.Schema{
				"user": {
					Type:        schema.TypeString,
					Required:    true,
					DefaultFunc: schema.EnvDefaultFunc("ZFS_PROVIDER_USERNAME", nil),
				},
				"host": {
					Type:        schema.TypeString,
					Required:    true,
					DefaultFunc: schema.EnvDefaultFunc("ZFS_PROVIDER_HOSTNAME", nil),
				},
				"port": {
					Type:        schema.TypeString,
					Required:    true,
					DefaultFunc: schema.EnvDefaultFunc("ZFS_PROVIDER_PORT", "22"),
				},
				"key": {
					Type:        schema.TypeString,
					Optional:    true,
					DefaultFunc: schema.EnvDefaultFunc("ZFS_PROVIDER_KEY", nil),
				},
				"key_path": {
					Type:        schema.TypeString,
					Optional:    true,
					DefaultFunc: schema.EnvDefaultFunc("ZFS_PROVIDER_KEY_PATH", nil),
				},
				"key_passphrase": {
					Type:        schema.TypeString,
					Optional:    true,
					DefaultFunc: schema.EnvDefaultFunc("ZFS_PROVIDER_KEY_PASSPHRASE", nil),
				},
				"password": {
					Type:        schema.TypeString,
					Optional:    true,
					DefaultFunc: schema.EnvDefaultFunc("ZFS_PROVIDER_PASSWORD", nil),
				},
				"command_prefix": {
					Description: "Can be used to prefix all ssh commands issued on the target host. For example, a command_prefix of 'sudo' can be used to elevate privileges on the target host, assuming password-less sudo is configured for the user",
					Type:        schema.TypeString,
					Optional:    true,
					DefaultFunc: schema.EnvDefaultFunc("ZFS_PROVIDER_COMMAND_PREFIX", nil),
				},
			},
			DataSourcesMap: map[string]*schema.Resource{
				"zfs_pool":    dataSourcePool(),
				"zfs_dataset": dataSourceDataset(),
			},
			ResourcesMap: map[string]*schema.Resource{
				"zfs_dataset": resourceDataset(),
			},
		}

		p.ConfigureContextFunc = configure(version, p)

		return p
	}
}

func configure(version string, p *schema.Provider) func(context.Context, *schema.ResourceData) (interface{}, diag.Diagnostics) {
	return func(ctx context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
		return &Config{
			command_prefix: d.Get("command_prefix").(string),
			ssh: &easyssh.MakeConfig{
				Server:     d.Get("host").(string),
				Port:       d.Get("port").(string),
				User:       d.Get("user").(string),
				Key:        d.Get("key").(string),
				KeyPath:    d.Get("key_path").(string),
				Password:   d.Get("password").(string),
				Passphrase: d.Get("key_passphrase").(string),
				Timeout:    60 * time.Second,
			},
		}, nil
	}
}
