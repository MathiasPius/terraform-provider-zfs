package provider

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/appleboy/easyssh-proxy"
)

func resourceDataset() *schema.Resource {
	return &schema.Resource{
		// This description is used by the documentation generator and the language server.
		Description: "zfs dataset resource.",

		CreateContext: resourceDatasetCreate,
		ReadContext:   resourceDatasetRead,
		UpdateContext: resourceDatasetUpdate,
		DeleteContext: resourceDatasetDelete,

		Schema: map[string]*schema.Schema{
			"name": {
				// This description is used by the documentation generator and the language server.
				Description: "Name of the ZFS dataset.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"mountpoint": {
				// This description is used by the documentation generator and the language server.
				Description: "Mountpoint of the dataset.",
				Type:        schema.TypeSet,
				MaxItems:    1,
				Optional:    true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"path": {
							Type:     schema.TypeString,
							Required: true,
						},
						"create": {
							Description: "If specified, will create the mountpoint if it does not exist",
							Type:        schema.TypeSet,
							MaxItems:    1,
							Optional:    true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"uid": {
										Type:     schema.TypeString,
										Optional: true,
									},
									"gid": {
										Type:     schema.TypeString,
										Optional: true,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func resourceDatasetCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	datasetName := d.Get("name").(string)

	ssh := meta.(*easyssh.MakeConfig)
	dataset, err := describeDataset(ssh, datasetName)

	if dataset != nil {
		// do comparison?
		log.Println("[DEBUG] zfs dataset exists!")
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

	dataset, err = createDataset(ssh, &CreateDataset{
		name:       datasetName,
		mountpoint: d.Get("mountpoint").(*schema.Set).List()[0].(map[string]interface{})["path"].(string),
	})

	if err != nil {
		diag.FromErr(err)
	}

	log.Printf("[DEBUG] committing guid: %s", dataset.guid)
	d.SetId(dataset.guid)

	return diags
}

func resourceDatasetRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	ssh := meta.(*easyssh.MakeConfig)

	datasetName := ""
	if id := d.Id(); id != "" {
		// If we have a Resource ID, then use that to lookup the real name
		// of the zfs resource, in case the name has changed.
		real_name, err := getDatasetNameByGuid(ssh, id)
		if err != nil {
			return diag.FromErr(err)
		}
		datasetName = *real_name
	} else {
		// If there's no Resource ID, then we can just assume the name
		// is accurate, and use that.
		datasetName = d.Get("name").(string)
	}

	dataset, err := describeDataset(ssh, datasetName)
	if err != nil {
		return diag.FromErr(err)
	}

	mountpoint := make([]map[string]string, 1)
	mountpoint[0] = map[string]string{
		"path": dataset.mountpoint,
	}

	d.Set("mountpoint", mountpoint)
	d.SetId(dataset.guid)

	return diags
}

func resourceDatasetUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	ssh := meta.(*easyssh.MakeConfig)
	datasetId := d.Id()
	old_name, err := getDatasetNameByGuid(ssh, datasetId)
	if err != nil {
		return diag.FromErr(err)
	}

	datasetName := d.Get("name").(string)
	d.Partial(true)

	if datasetName != *old_name {
		if err := renameDataset(ssh, *old_name, datasetName); err != nil {
			return diag.FromErr(err)
		}
	}

	// Change mountpoint
	if d.HasChange("mountpoint") {
		log.Println("[DEBUG] updating path!")
		new_mountpoint := d.Get("mountpoint").(*schema.Set).List()[0].(map[string]interface{})["path"].(string)
		if _, err := updateOption(ssh, datasetName, "mountpoint", new_mountpoint); err != nil {
			return diag.FromErr(err)
		}
	}

	d.Partial(false)
	return resourceDatasetRead(ctx, d, meta)
}

func resourceDatasetDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	ssh := meta.(*easyssh.MakeConfig)
	datasetName := d.Get("name").(string)

	if err := destroyDataset(ssh, datasetName); err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")

	return diags
}
