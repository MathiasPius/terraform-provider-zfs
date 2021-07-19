package provider

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/appleboy/easyssh-proxy"
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
				// This description is used by the documentation generator and the language server.
				Description: "Mountpoint of the dataset.",
				Type:        schema.TypeList,
				Computed:    true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"path": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"owner": {
							Description: "Set owner of the mountpoint. Must be a valid username",
							Type:        schema.TypeString,
							Computed:    true,
						},
						"uid": {
							Description: "Set owner of the mountpoint. Must be a valid uid",
							Type:        schema.TypeString,
							Computed:    true,
						},
						"group": {
							Description: "Set group of the mountpoint. Must be a valid group name",
							Type:        schema.TypeString,
							Computed:    true,
						},
						"gid": {
							Description: "Set group of the mountpoint. Must be a valid gid",
							Type:        schema.TypeString,
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func dataSourceDatasetRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	ssh := meta.(*easyssh.MakeConfig)

	datasetName := d.Get("name").(string)
	dataset, err := describeDataset(ssh, datasetName)

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

	owner, err := getFileOwnership(ssh, dataset.mountpoint)
	if err != nil {
		return diag.FromErr(err)
	}

	mountpoint := make([]map[string]string, 1)
	mountpoint[0] = map[string]string{
		"path":  dataset.mountpoint,
		"owner": owner.userName,
		"group": owner.groupName,
		"uid":   owner.uid,
		"gid":   owner.gid,
	}

	d.Set("mountpoint", mountpoint)
	d.SetId(dataset.guid)

	return diags
}
