package provider

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceFilesystem() *schema.Resource {
	return &schema.Resource{
		// This description is used by the documentation generator and the language server.
		Description: "Data about a specific filesystem.",

		ReadContext: dataSourceFilesystemRead,

		Schema: map[string]*schema.Schema{
			"name": {
				// This description is used by the documentation generator and the language server.
				Description: "Name of the zfs filesystem.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"mountpoint": {
				Description: "Mountpoint of the filesystem.",
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

func dataSourceFilesystemRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	config := meta.(*Config)

	filesystemName := d.Get("name").(string)
	filesystem, err := describeDataset(config, filesystemName, getPropertyNames(d))

	if filesystem == nil {
		log.Println("[DEBUG] zfs filesystem does not exist!")
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

	d.SetId(filesystem.guid)

	if filesystem.mountpoint != "" && filesystem.mountpoint != "none" && filesystem.mountpoint != "legacy" {
		owner, err := getFileOwnership(config, filesystem.mountpoint)
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

		if err = d.Set("mountpoint", filesystem.mountpoint); err != nil {
			return diag.FromErr(err)
		}
	}

	if err = updateCalculatedPropertiesInState(d, filesystem.properties); err != nil {
		return diag.FromErr(err)
	}

	return diags
}
