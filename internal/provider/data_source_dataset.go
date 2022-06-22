package provider

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceDataset() *schema.Resource {
	return &schema.Resource{
		// This description is used by the documentation generator and the language server.
		Description: "Data about a specific dataset.",

		ReadContext: dataSourceDatasetRead,

		Schema: map[string]*schema.Schema{
			"name": {
				// This description is used by the documentation generator and the language server.
				Description: "Name of the zfs dataset.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"mountpoint": {
				Description: "Mountpoint of the dataset.",
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
		},
	}
}

func dataSourceDatasetRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	config := meta.(*Config)

	datasetName := d.Get("name").(string)
	dataset, err := describeDataset(config, datasetName)

	if dataset == nil {
		log.Println("[DEBUG] zfs dataset does not exist!")
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

	d.SetId(dataset.guid)

	if dataset.mountpoint != "" && dataset.mountpoint != "none" && dataset.mountpoint != "legacy" {
		owner, err := getFileOwnership(config, dataset.mountpoint)
		if err != nil {
			return diag.FromErr(err)
		}

		d.Set("owner", owner.userName)
		d.Set("group", owner.groupName)
		d.Set("uid", owner.uid)
		d.Set("gid", owner.gid)
		d.Set("mountpoint", dataset.mountpoint)
	}

	return diags
}
