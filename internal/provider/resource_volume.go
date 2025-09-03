package provider

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceVolume() *schema.Resource {
	return &schema.Resource{
		// This description is used by the documentation generator and the language server.
		Description: "zfs volume resource.",

		CreateContext: resourceVolumeCreate,
		ReadContext:   resourceVolumeRead,
		UpdateContext: resourceVolumeUpdate,
		DeleteContext: resourceVolumeDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				// This description is used by the documentation generator and the language server.
				Description: "Name of the ZFS volume.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"mountpoint": {
				Description: "Mountpoint of the volume.",
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
			"property":       &propertySchema,
			"property_mode":  &propertyModeSchema,
			"properties":     &propertiesSchema,
			"raw_properties": &rawPropertiesSchema,
		},
	}
}

func resourceVolumeCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	config := meta.(*Config)

	volumeName := d.Get("name").(string)
	volume, err := describeDataset(config, volumeName, getPropertyNames(d))

	if volume != nil {
		log.Printf("[DEBUG] zfs volume %s already exists!", volumeName)
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
	properties := parsePropertyBlocks(d.Get("property").(*schema.Set).List())
	volume, err = createDataset(config, &CreateDataset{
		name:       volumeName,
		mountpoint: mountpoint,
		properties: properties,
	})

	if err != nil {
		return diag.FromErr(err)
	}

	// We're setting the ID here because the volume DOES exist, even if the mountpoint
	// is not properly configured!
	log.Printf("[DEBUG] committing guid: %s", volume.guid)
	d.SetId(volume.guid)

	if mountpoint != "none" && mountpoint != "legacy" {
		if uid, ok := d.GetOk("uid"); ok {
			if _, err = callSshCommand(config, "chown '%d' '%s'", uid.(int), mountpoint); err != nil {
				return diag.FromErr(err)
			}
		}

		if gid, ok := d.GetOk("gid"); ok {
			if _, err = callSshCommand(config, "chgrp '%d' '%s'", gid.(int), mountpoint); err != nil {
				return diag.FromErr(err)
			}
		}

		if owner, ok := d.GetOk("owner"); ok {
			if _, err = callSshCommand(config, "chown '%s' '%s'", owner.(string), mountpoint); err != nil {
				return diag.FromErr(err)
			}
		}

		if group, ok := d.GetOk("group"); ok {
			if _, err = callSshCommand(config, "chgrp '%s' '%s'", group.(string), mountpoint); err != nil {
				return diag.FromErr(err)
			}
		}
	}

	return diags
}

func resourceVolumeRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	config := meta.(*Config)

	volumeName := d.Get("name").(string)
	if id := d.Id(); id != "" {
		// If we have a Resource ID, then use that to lookup the real name
		// of the zfs resource, in case the name has changed.
		real_name, err := getDatasetNameByGuid(config, id)
		if err != nil {
			return diag.FromErr(fmt.Errorf("the volume %s identified by guid %s could not be found. It was likely deleted on the server outside of terraform", volumeName, id))
		}
		volumeName = *real_name
	}

	if err := d.Set("name", volumeName); err != nil {
		return diag.FromErr(err)
	}

	volume, err := describeDataset(config, volumeName, getPropertyNames(d))
	if err != nil {
		return diag.FromErr(err)
	}

	if err = d.Set("mountpoint", volume.mountpoint); err != nil {
		return diag.FromErr(err)
	}

	if volume.mountpoint != "none" && volume.mountpoint != "legacy" {
		log.Println("[DEBUG] Fetching volume mountpoint ownership information")
		ownership, err := getFileOwnership(config, volume.mountpoint)
		if err != nil {
			return diag.FromErr(err)
		}

		// Ignore any values not explicitly tracked by terraform
		if _, ok := d.GetOk("owner"); ok {
			if err = d.Set("owner", ownership.userName); err != nil {
				return diag.FromErr(err)
			}
		}

		if _, ok := d.GetOk("group"); ok {
			if err = d.Set("group", ownership.groupName); err != nil {
				return diag.FromErr(err)
			}
		}

		if _, ok := d.GetOk("gid"); ok {
			if err = d.Set("gid", ownership.gid); err != nil {
				return diag.FromErr(err)
			}
		}

		if _, ok := d.GetOk("uid"); ok {
			if err = d.Set("uid", ownership.uid); err != nil {
				return diag.FromErr(err)
			}
		}
	} else {
		if err = d.Set("owner", nil); err != nil {
			return diag.FromErr(err)
		}

		if err = d.Set("group", nil); err != nil {
			return diag.FromErr(err)
		}

		if err = d.Set("gid", nil); err != nil {
			return diag.FromErr(err)
		}

		if err = d.Set("uid", nil); err != nil {
			return diag.FromErr(err)
		}
	}

	if err := updatePropertiesInState(d, volume.properties, []string{"mountpoint"}); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(volume.guid)
	return diags
}

func resourceVolumeUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := meta.(*Config)
	old_name, err := getDatasetNameByGuid(config, d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	volumeName := d.Get("name").(string)
	// Rename the volume
	if volumeName != *old_name {
		if err := renameDataset(config, *old_name, volumeName); err != nil {
			return diag.FromErr(err)
		}
	}

	volume, err := describeDataset(config, volumeName, getPropertyNames(d))
	if err != nil {
		return diag.FromErr(err)
	}

	overrideProperties := map[string]string{"mountpoint": d.Get("mountpoint").(string)}
	err = applyPropertyDiff(config, d, volumeName, volume.properties, overrideProperties)
	if err != nil {
		return diag.FromErr(err)
	}

	if mountpoint, ok := d.GetOk("mountpoint"); ok {
		if uid, ok := d.GetOk("uid"); ok && d.HasChange("uid") {
			if _, err = callSshCommand(config, "chown '%d' '%s'", uid.(int), mountpoint.(string)); err != nil {
				return diag.FromErr(err)
			}
		}

		if gid, ok := d.GetOk("gid"); ok && d.HasChange("gid") {
			if _, err = callSshCommand(config, "chgrp '%d' '%s'", gid.(int), mountpoint.(string)); err != nil {
				return diag.FromErr(err)
			}
		}

		if owner, ok := d.GetOk("owner"); ok && d.HasChange("owner") {
			if _, err = callSshCommand(config, "chown '%s' '%s'", owner.(string), mountpoint.(string)); err != nil {
				return diag.FromErr(err)
			}
		}

		if group, ok := d.GetOk("group"); ok && d.HasChange("group") {
			if _, err = callSshCommand(config, "chgrp '%s' '%s'", group.(string), mountpoint.(string)); err != nil {
				return diag.FromErr(err)
			}
		}
	}

	return resourceVolumeRead(ctx, d, meta)
}

func resourceVolumeDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	config := meta.(*Config)
	volumeName := d.Get("name").(string)

	if err := destroyDataset(config, volumeName); err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")

	return diags
}
