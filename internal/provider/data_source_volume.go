package provider

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceVolume() *schema.Resource {
	return &schema.Resource{
		// This description is used by the documentation generator and the language server.
		Description: "Data about a specific volume.",

		ReadContext: dataSourceVolumeRead,

		Schema: map[string]*schema.Schema{
			"name": {
				// This description is used by the documentation generator and the language server.
				Description: "Name of the zfs volume.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"volsize": {
				Description: "Size of the volume.",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"properties":     &propertiesSchema,
			"raw_properties": &rawPropertiesSchema,
		},
	}
}

func dataSourceVolumeRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	config := meta.(*Config)

	volumeName := d.Get("name").(string)
	volume, err := describeDataset(config, volumeName, getPropertyNames(d))

	if volume == nil {
		log.Println("[DEBUG] zfs volume does not exist!")
	}

	if err != nil {
		switch err := err.(type) {
		case *DatasetError:
			{
				if err.errmsg != "dataset does not exist" {
					log.Printf("[DEBUG] zfs err: %s", err.Error())
					return diag.FromErr(err)
				}
			}
		default:
			{
				log.Printf("[DEBUG] zfs err: %s", err.Error())
				return diag.FromErr(err)
			}
		}
	}

	d.SetId(volume.guid)

	if err = updateCalculatedPropertiesInState(d, volume.properties); err != nil {
		return diag.FromErr(err)
	}

	return diags
}
