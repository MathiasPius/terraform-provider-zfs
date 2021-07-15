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
		switch err.(type) {
		case *DatasetError:
			{
				if err.(*DatasetError).errmsg != "dataset does not exist" {
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

	_, err = createDataset(ssh, &CreateDataset{
		name:       datasetName,
		mountpoint: d.Get("mountpoint.0.path").(string),
	})

	if err != nil {
		diag.FromErr(err)
	}

	d.SetId(datasetName)

	return diags
}

func resourceDatasetRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	datasetName := d.Get("name").(string)

	ssh := meta.(*easyssh.MakeConfig)
	_, err := describeDataset(ssh, datasetName)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(datasetName)

	return diags
}

func resourceDatasetUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	ssh := meta.(*easyssh.MakeConfig)
	datasetName := d.Get("name").(string)

	d.Partial(true)

	// Change mountpoint
	if d.HasChange("mountpoint.0.path") {
		if _, err := updateOption(ssh, datasetName, "mountpoint", d.Get("mountpoint.0.path").(string)); err != nil {
			return diag.FromErr(err)
		}
	}

	d.Partial(false)
	return diags
}

func resourceDatasetDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	// use the meta value to retrieve your client from the provider configure method
	// client := meta.(*apiClient)

	return diag.Errorf("not implemented")
}
