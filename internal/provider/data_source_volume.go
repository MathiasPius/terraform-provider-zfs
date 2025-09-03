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
			"mountpoint": {
				Description: "Mountpoint of the volume.",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"owner": {
				Description: "Username of the owner of the mountpoint",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"uid": {
				Description: "uid of the owner of the mountpoint",
				Type:        schema.TypeInt,
				Computed:    true,
			},
			"group": {
				Description: "Name of the group owning the mountpoint",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"gid": {
				Description: "gid of the group owning the mountpoint.",
				Type:        schema.TypeInt,
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

	if volume.mountpoint != "" && volume.mountpoint != "none" && volume.mountpoint != "legacy" {
		owner, err := getFileOwnership(config, volume.mountpoint)
		if err != nil {
			return diag.FromErr(err)
		}

		if err := d.Set("owner", owner.userName); err != nil {
			return diag.FromErr(err)
		}

		if err = d.Set("group", owner.groupName); err != nil {
			return diag.FromErr(err)
		}

		if err = d.Set("uid", owner.uid); err != nil {
			return diag.FromErr(err)
		}

		if err = d.Set("gid", owner.gid); err != nil {
			return diag.FromErr(err)
		}

		if err = d.Set("mountpoint", volume.mountpoint); err != nil {
			return diag.FromErr(err)
		}
	}

	if err = updateCalculatedPropertiesInState(d, volume.properties); err != nil {
		return diag.FromErr(err)
	}

	return diags
}
