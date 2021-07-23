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
				Description: "Mountpoint of the dataset.",
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "none",
			},
			"owner": {
				Description:   "Set owner of the mountpoint. Must be a valid username",
				Type:          schema.TypeString,
				Optional:      true,
				ConflictsWith: []string{"uid"},
				RequiredWith:  []string{"mountpoint"},
			},
			"uid": {
				Description:   "Set owner of the mountpoint. Must be a valid uid",
				Type:          schema.TypeInt,
				Optional:      true,
				ConflictsWith: []string{"owner"},
				RequiredWith:  []string{"mountpoint"},
			},
			"group": {
				Description:   "Set group of the mountpoint. Must be a valid group name",
				Type:          schema.TypeString,
				Optional:      true,
				ConflictsWith: []string{"gid"},
				RequiredWith:  []string{"mountpoint"},
			},
			"gid": {
				Description:   "Set group of the mountpoint. Must be a valid gid",
				Type:          schema.TypeInt,
				Optional:      true,
				ConflictsWith: []string{"group"},
				RequiredWith:  []string{"mountpoint"},
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
		log.Printf("[DEBUG] zfs dataset %s already exists!", datasetName)
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

	mountpoint := d.Get("mountpoint").(string)
	dataset, err = createDataset(ssh, &CreateDataset{
		name:       datasetName,
		mountpoint: mountpoint,
	})

	if err != nil {
		return diag.FromErr(err)
	}

	// We're setting the ID here because the dataset DOES exist, even if the mountpoint
	// is not properly configured!
	log.Printf("[DEBUG] committing guid: %s", dataset.guid)
	d.SetId(dataset.guid)

	if mountpoint != "none" && mountpoint != "legacy" {
		if uid, ok := d.GetOk("uid"); ok {
			if _, err = callSshCommand(ssh, "sudo chown '%d' '%s'", uid.(int), mountpoint); err != nil {
				return diag.FromErr(err)
			}
		}

		if gid, ok := d.GetOk("gid"); ok {
			if _, err = callSshCommand(ssh, "sudo chgrp '%d' '%s'", gid.(int), mountpoint); err != nil {
				return diag.FromErr(err)
			}
		}

		if owner, ok := d.GetOk("owner"); ok {
			if _, err = callSshCommand(ssh, "sudo chown '%s' '%s'", owner.(string), mountpoint); err != nil {
				return diag.FromErr(err)
			}
		}

		if group, ok := d.GetOk("group"); ok {
			if _, err = callSshCommand(ssh, "sudo chgrp '%s' '%s'", group.(string), mountpoint); err != nil {
				return diag.FromErr(err)
			}
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
			return diag.FromErr(fmt.Errorf("the dataset %s identified by guid %s could not be found. It was likely deleted on the server outside of terraform", datasetName, id))
		}
		datasetName = *real_name
	}
	d.Set("name", datasetName)

	dataset, err := describeDataset(ssh, datasetName)
	if err != nil {
		return diag.FromErr(err)
	}

	d.Set("mountpoint", dataset.mountpoint)

	if dataset.mountpoint != "none" && dataset.mountpoint != "legacy" {
		log.Println("[DEBUG] Fetching dataset mountpoint ownership information")
		ownership, err := getFileOwnership(ssh, dataset.mountpoint)
		if err != nil {
			return diag.FromErr(err)
		}

		// Ignore any values not explicitly tracked by terraform
		if _, ok := d.GetOk("owner"); ok {
			d.Set("owner", ownership.userName)
		}

		if _, ok := d.GetOk("group"); ok {
			d.Set("group", ownership.groupName)
		}

		if _, ok := d.GetOk("gid"); ok {
			d.Set("gid", ownership.gid)
		}

		if _, ok := d.GetOk("uid"); ok {
			d.Set("uid", ownership.uid)
		}
	} else {
		d.Set("owner", nil)
		d.Set("group", nil)
		d.Set("gid", nil)
		d.Set("uid", nil)
	}

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
	// Rename the dataset
	if datasetName != *old_name {
		if err := renameDataset(ssh, *old_name, datasetName); err != nil {
			return diag.FromErr(err)
		}
	}

	if mountpoint, ok := d.GetOk("mountpoint"); ok {
		if d.HasChange("mountpoint") {
			if _, err := updateOption(ssh, datasetName, "mountpoint", mountpoint.(string)); err != nil {
				return diag.FromErr(err)
			}

			if uid, ok := d.GetOk("uid"); ok {
				if _, err = callSshCommand(ssh, "sudo chown '%d' '%s'", uid.(int), mountpoint.(string)); err != nil {
					return diag.FromErr(err)
				}
			}

			if gid, ok := d.GetOk("gid"); ok {
				if _, err = callSshCommand(ssh, "sudo chgrp '%d' '%s'", gid.(int), mountpoint.(string)); err != nil {
					return diag.FromErr(err)
				}
			}

			if owner, ok := d.GetOk("owner"); ok {
				if _, err = callSshCommand(ssh, "sudo chown '%s' '%s'", owner.(string), mountpoint.(string)); err != nil {
					return diag.FromErr(err)
				}
			}

			if group, ok := d.GetOk("group"); ok {
				if _, err = callSshCommand(ssh, "sudo chgrp '%s' '%s'", group.(string), mountpoint.(string)); err != nil {
					return diag.FromErr(err)
				}
			}
		} else {
			if uid, ok := d.GetOk("uid"); d.HasChange("uid") && ok {
				if _, err = callSshCommand(ssh, "sudo chgrp '%d' '%s'", uid.(int), mountpoint.(string)); err != nil {
					return diag.FromErr(err)
				}
			}
		}

		if gid, ok := d.GetOk("gid"); d.HasChange("gid") && ok {
			if _, err = callSshCommand(ssh, "sudo chgrp '%d' '%s'", gid.(int), mountpoint.(string)); err != nil {
				return diag.FromErr(err)
			}
		}

		if owner, ok := d.GetOk("owner"); d.HasChange("owner") && ok {
			if _, err = callSshCommand(ssh, "sudo chown '%s' '%s'", owner.(string), mountpoint.(string)); err != nil {
				return diag.FromErr(err)
			}
		}

		if group, ok := d.GetOk("group"); d.HasChange("group") && ok {
			if _, err = callSshCommand(ssh, "sudo chgrp '%s' '%s'", group.(string), mountpoint.(string)); err != nil {
				return diag.FromErr(err)
			}
		}
	}

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
