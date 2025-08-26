package provider

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceFilesystem() *schema.Resource {
	return &schema.Resource{
		// This description is used by the documentation generator and the language server.
		Description: "zfs filesystem resource.",

		CreateContext: resourceFilesystemCreate,
		ReadContext:   resourceFilesystemRead,
		UpdateContext: resourceFilesystemUpdate,
		DeleteContext: resourceFilesystemDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				// This description is used by the documentation generator and the language server.
				Description: "Name of the ZFS filesystem.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"mountpoint": {
				Description: "Mountpoint of the filesystem.",
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

func resourceFilesystemCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	config := meta.(*Config)

	filesystemName := d.Get("name").(string)
	filesystem, err := describeDataset(config, filesystemName, getPropertyNames(d))

	if filesystem != nil {
		log.Printf("[DEBUG] zfs filesystem %s already exists!", filesystemName)
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
	filesystem, err = createDataset(config, &CreateDataset{
		name:       filesystemName,
		mountpoint: mountpoint,
		properties: properties,
	})

	if err != nil {
		return diag.FromErr(err)
	}

	// We're setting the ID here because the filesystem DOES exist, even if the mountpoint
	// is not properly configured!
	log.Printf("[DEBUG] committing guid: %s", filesystem.guid)
	d.SetId(filesystem.guid)

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

func resourceFilesystemRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	config := meta.(*Config)

	filesystemName := d.Get("name").(string)
	if id := d.Id(); id != "" {
		// If we have a Resource ID, then use that to lookup the real name
		// of the zfs resource, in case the name has changed.
		real_name, err := getDatasetNameByGuid(config, id)
		if err != nil {
			return diag.FromErr(fmt.Errorf("the filesystem %s identified by guid %s could not be found. It was likely deleted on the server outside of terraform", filesystemName, id))
		}
		filesystemName = *real_name
	}

	if err := d.Set("name", filesystemName); err != nil {
		return diag.FromErr(err)
	}

	filesystem, err := describeDataset(config, filesystemName, getPropertyNames(d))
	if err != nil {
		return diag.FromErr(err)
	}

	if err = d.Set("mountpoint", filesystem.mountpoint); err != nil {
		return diag.FromErr(err)
	}

	if filesystem.mountpoint != "none" && filesystem.mountpoint != "legacy" {
		log.Println("[DEBUG] Fetching filesystem mountpoint ownership information")
		ownership, err := getFileOwnership(config, filesystem.mountpoint)
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

	if err := updatePropertiesInState(d, filesystem.properties, []string{"mountpoint"}); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(filesystem.guid)
	return diags
}

func resourceFilesystemUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := meta.(*Config)
	old_name, err := getDatasetNameByGuid(config, d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	filesystemName := d.Get("name").(string)
	// Rename the filesystem
	if filesystemName != *old_name {
		if err := renameDataset(config, *old_name, filesystemName); err != nil {
			return diag.FromErr(err)
		}
	}

	filesystem, err := describeDataset(config, filesystemName, getPropertyNames(d))
	if err != nil {
		return diag.FromErr(err)
	}

	overrideProperties := map[string]string{"mountpoint": d.Get("mountpoint").(string)}
	err = applyPropertyDiff(config, d, filesystemName, filesystem.properties, overrideProperties)
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

	return resourceFilesystemRead(ctx, d, meta)
}

func resourceFilesystemDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	config := meta.(*Config)
	filesystemName := d.Get("name").(string)

	if err := destroyDataset(config, filesystemName); err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")

	return diags
}
