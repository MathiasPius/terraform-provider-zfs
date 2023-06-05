package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
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
			"properties":     &propertiesSchema,
			"raw_properties": &rawPropertiesSchema,
		},
	}
}

func dataSourcePoolRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	// use the meta value to retrieve your client from the provider configure method
	// client := meta.(*apiClient)
	var diags diag.Diagnostics

	config := meta.(*Config)

	poolName := d.Get("name").(string)

	pool, err := describePool(config, poolName, getPropertyNames(d))
	if err != nil {
		return diag.FromErr(err)
	}

	if err = updateCalculatedPropertiesInState(d, pool.properties); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(pool.guid)

	return diags
}
