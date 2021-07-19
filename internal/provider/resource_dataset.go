package provider

import (
	"context"
	"fmt"
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

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

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
				Type:        schema.TypeList,
				MaxItems:    1,
				Optional:    true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"path": {
							Type:     schema.TypeString,
							Required: true,
						},
						"owner": {
							Description:   "Set owner of the mountpoint. Must be a valid username",
							Type:          schema.TypeString,
							Optional:      true,
							Computed:      true,
							ConflictsWith: []string{"mountpoint.uid"},
						},
						"uid": {
							Description:   "Set owner of the mountpoint. Must be a valid uid",
							Type:          schema.TypeString,
							Optional:      true,
							Computed:      true,
							ConflictsWith: []string{"mountpoint.owner"},
						},
						"group": {
							Description:   "Set group of the mountpoint. Must be a valid group name",
							Type:          schema.TypeString,
							Optional:      true,
							Computed:      true,
							ConflictsWith: []string{"mountpoint.gid"},
						},
						"gid": {
							Description:   "Set group of the mountpoint. Must be a valid gid",
							Type:          schema.TypeString,
							Optional:      true,
							Computed:      true,
							ConflictsWith: []string{"mountpoint.group"},
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
		mountpoint: d.Get("mountpoint").([]interface{})[0].(map[string]interface{})["path"].(string),
	})

	if err != nil {
		return diag.FromErr(err)
	}

	// We're setting the ID here because the dataset DOES exist, even if the mountpoint
	// is not properly configured!
	log.Printf("[DEBUG] committing guid: %s", dataset.guid)
	d.SetId(dataset.guid)

	if uid := d.Get("mountpoint.uid"); uid != nil {
		log.Printf("[DEBUG] setting user id (%s) of dataset (%s) mountpoint path", uid, datasetName)
		if _, err = callSshCommand(ssh, "sudo chown '%s' '%s'", uid.(string), dataset.mountpoint); err != nil {
			return diag.FromErr(err)
		}
	}

	if gid := d.Get("mountpoint.gid"); gid != nil {
		log.Printf("[DEBUG] setting group id (%s) of dataset (%s) mountpoint path", gid, datasetName)
		if _, err = callSshCommand(ssh, "sudo chgrp '%s' '%s'", gid.(string), dataset.mountpoint); err != nil {
			return diag.FromErr(err)
		}
	}

	if owner := d.Get("mountpoint.owner"); owner != nil {
		log.Printf("[DEBUG] setting owner (%s) of dataset (%s) mountpoint path", owner, datasetName)
		if _, err = callSshCommand(ssh, "sudo chown '%s' '%s'", owner.(string), dataset.mountpoint); err != nil {
			return diag.FromErr(err)
		}
	}

	if group := d.Get("mountpoint.group"); group != nil {
		log.Printf("[DEBUG] setting group name (%s) of dataset (%s) mountpoint path", group, datasetName)
		if _, err = callSshCommand(ssh, "sudo chgrp '%s' '%s'", group.(string), dataset.mountpoint); err != nil {
			return diag.FromErr(err)
		}
	}

	return diags
}

func resourceDatasetRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	ssh := meta.(*easyssh.MakeConfig)

	datasetName := d.Get("name").(string)
	if id := d.Id(); id != "" {
		// If we have a Resource ID, then use that to lookup the real name
		// of the zfs resource, in case the name has changed.
		real_name, err := getDatasetNameByGuid(ssh, id)
		if err != nil {
			return diag.FromErr(fmt.Errorf("the dataset %s identified by guid %s could not be found. It was likely deleted on the server outside of terraform.", datasetName, id))
		}
		datasetName = *real_name
	}
	d.Set("name", datasetName)

	dataset, err := describeDataset(ssh, datasetName)
	if err != nil {
		return diag.FromErr(err)
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

func resourceDatasetUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	ssh := meta.(*easyssh.MakeConfig)
	old_name, err := getDatasetNameByGuid(ssh, d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	datasetName := d.Get("name").(string)
	d.Partial(true)

	// Rename the dataset
	if datasetName != *old_name {
		if err := renameDataset(ssh, *old_name, datasetName); err != nil {
			return diag.FromErr(err)
		}
	}

	// Change mountpoint
	if d.HasChange("mountpoint") {
		mountpoint := d.Get("mountpoint").([]interface{})[0].(map[string]interface{})["path"].(string)
		log.Println("[DEBUG] updating path!")
		if _, err := updateOption(ssh, datasetName, "mountpoint", mountpoint); err != nil {
			return diag.FromErr(err)
		}

		// If the value has changed, and is now nil, then it should be reset to default
		uid := d.Get("mountpoint").([]interface{})[0].(map[string]interface{})["uid"]
		if uid == nil {
			uid = "0"
		} else {
			uid = uid.(string)
		}
		log.Printf("[DEBUG] setting user id (%s) of dataset (%s) mountpoint path", uid, datasetName)
		if _, err = callSshCommand(ssh, "sudo chown '%s' '%s'", uid, mountpoint); err != nil {
			return diag.FromErr(err)
		}

		// If the value has changed, and is now nil, then it should be reset to default
		gid := d.Get("mountpoint").([]interface{})[0].(map[string]interface{})["gid"]
		if gid == nil {
			gid = "0"
		} else {
			gid = gid.(string)
		}
		log.Printf("[DEBUG] setting group id (%s) of dataset (%s) mountpoint path", gid, datasetName)
		if _, err = callSshCommand(ssh, "sudo chgrp '%s' '%s'", gid, mountpoint); err != nil {
			return diag.FromErr(err)
		}

		owner := d.Get("mountpoint").([]interface{})[0].(map[string]interface{})["owner"]
		if owner == nil {
			owner = "0"
		} else {
			owner = owner.(string)
		}
		log.Printf("[DEBUG] setting owner (%s) of dataset (%s) mountpoint path", owner, datasetName)
		if _, err = callSshCommand(ssh, "sudo chown '%s' '%s'", owner, mountpoint); err != nil {
			return diag.FromErr(err)
		}

		group := d.Get("mountpoint").([]interface{})[0].(map[string]interface{})["group"]
		if group == nil {
			group = "0"
		} else {
			group = group.(string)
		}
		log.Printf("[DEBUG] setting group name (%s) of dataset (%s) mountpoint path", group, datasetName)
		if _, err = callSshCommand(ssh, "sudo chgrp '%s' '%s'", group, mountpoint); err != nil {
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
