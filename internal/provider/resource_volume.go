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
			"volsize": {
				Description: "Size of the volume.",
				Type:        schema.TypeString,
				Optional:    false,
				Required:    true,
			},
			"sparse": {
				Description: "If the volume is sparsely provisioned. Defaults to `false`",
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
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

	volsize := d.Get("volsize").(string)
	sparse := d.Get("sparse").(bool)
	properties := parsePropertyBlocks(d.Get("property").(*schema.Set).List())
	volume, err = createDataset(config, &CreateDataset{
		dsType:     VolumeType,
		name:       volumeName,
		volsize:    volsize,
		sparse:     sparse,
		properties: properties,
	})

	if err != nil {
		return diag.FromErr(err)
	}

	// We're setting the ID here because the volume DOES exist, even if the mountpoint
	// is not properly configured!
	log.Printf("[DEBUG] committing guid: %s", volume.guid)
	d.SetId(volume.guid)

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

	if err = d.Set("volsize", volume.volsize); err != nil {
		return diag.FromErr(err)
	}

	if err := updatePropertiesInState(d, volume.properties, []string{"volsize"}); err != nil {
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

	overrideProperties := map[string]string{"volsize": d.Get("volsize").(string)}
	err = applyPropertyDiff(config, d, volumeName, volume.properties, overrideProperties)
	if err != nil {
		return diag.FromErr(err)
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
