package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/appleboy/easyssh-proxy"
)

func dataSourcePool() *schema.Resource {
	return &schema.Resource{
		// This description is used by the documentation generator and the language server.
		Description: "Sample data source in the Terraform provider scaffolding.",

		ReadContext: dataSourcePoolRead,

		Schema: map[string]*schema.Schema{
			"name": {
				// This description is used by the documentation generator and the language server.
				Description: "Name of the zpool.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"size": {
				Description: "Size of the pool.",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"capacity": {
				Description: "Capacity of the pool.",
				Type:        schema.TypeString,
				Computed:    true,
			},
		},
	}
}

func dataSourcePoolRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	// use the meta value to retrieve your client from the provider configure method
	// client := meta.(*apiClient)
	var diags diag.Diagnostics

	ssh := meta.(*easyssh.MakeConfig)

	poolName := d.Get("name").(string)

	pool, err := describePool(ssh, poolName)
	if err != nil {
		return diag.FromErr(err)
	}

	d.Set("name", poolName)
	d.Set("size", pool.size)
	d.Set("capacity", pool.capacity)
	d.SetId(pool.guid)

	return diags
}
