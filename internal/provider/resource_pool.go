package provider

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

var vdevSchema = &schema.Resource{
	Schema: map[string]*schema.Schema{
		"path": {
			Type:        schema.TypeString,
			Description: "Device path of the vdev to add",
			Required:    true,
			ForceNew:    true,
		},
	},
}

var mirrorSchema = &schema.Resource{
	Schema: map[string]*schema.Schema{
		"device": {
			Description: "Device(s) which make up the mirror. Repeat the block for multiple devices",
			Type:        schema.TypeSet,
			Required:    true,
			ForceNew:    true,
			Elem:        vdevSchema,
			MinItems:    2,
		},
	},
}

type Device struct {
	path string
}

type Mirror struct {
	device []Device
}

func resourcePool() *schema.Resource {
	return &schema.Resource{
		// This description is used by the documentation generator and the language server.
		Description: "zfs pool resource.",

		CreateContext: resourcePoolCreate,
		ReadContext:   resourcePoolRead,
		UpdateContext: resourcePoolUpdate,
		DeleteContext: resourcePoolDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Description: "Name of the zpool.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"mirror": {
				Description: "Defines a mirrored vdev",
				Type:        schema.TypeSet,
				Optional:    true,
				Elem:        mirrorSchema,
			},
			"device": {
				Description: "Defines a striped vdev",
				Type:        schema.TypeSet,
				Optional:    true,
				AtLeastOneOf: []string{
					"device", "mirror",
				},
				ConflictsWith: []string{
					"mirror",
				},
				Elem: vdevSchema,
			},
		},
	}
}

func resourcePoolCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	poolName := d.Get("name").(string)

	config := meta.(*Config)

	pool, err := describePool(config, poolName)
	if pool != nil {
		log.Printf("[DEBUG] zpool %s already exists!", poolName)
	}

	log.Printf("[DEBUG] check: %s, %s", pool, err)

	if err != nil {
		switch err := err.(type) {
		case *PoolError:
			{
				if err.errmsg != "zpool does not exist" {
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

	vdev_spec, err := parseVdevSpecification(d.Get("mirror"), d.Get("devices"))
	if err != nil {
		log.Printf("[DEBUG] failed to parse vdev specification")
		return diag.FromErr(err)
	}

	pool, err = createPool(config, &CreatePool{
		name:  poolName,
		vdevs: vdev_spec,
	})

	if err != nil {
		return diag.FromErr(err)
	}

	// We're setting the ID here because the dataset DOES exist, even if the mountpoint
	// is not properly configured!
	log.Printf("[DEBUG] committing guid: %s", pool.guid)
	d.SetId(pool.guid)

	return diags
}

func resourcePoolRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	config := meta.(*Config)

	poolName := d.Get("name").(string)
	if id := d.Id(); id != "" {
		// If we have a Resource ID, then use that to lookup the real name
		// of the zfs resource, in case the name has changed.
		real_name, err := getPoolNameByGuid(config, id)
		if err != nil {
			return diag.FromErr(fmt.Errorf("the zpool %s identified by guid %s could not be found. It was likely deleted on the server outside of terraform", poolName, id))
		}
		poolName = *real_name
	}
	d.Set("name", poolName)

	pool, err := describePool(config, poolName)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(pool.guid)
	return diags
}

func resourcePoolUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	/*
		config := meta.(*Config)
			old_name, err := getDatasetNameByGuid(config, d.Id())
			if err != nil {
				return diag.FromErr(err)
			}

			datasetName := d.Get("name").(string)
			// Rename the dataset
			if datasetName != *old_name {
				if err := renameDataset(config, *old_name, datasetName); err != nil {
					return diag.FromErr(err)
				}
			}
	*/
	return resourcePoolRead(ctx, d, meta)
}

func resourcePoolDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	/*
		config := meta.(*Config)
		datasetName := d.Get("name").(string)

		if err := destroyDataset(config, datasetName); err != nil {
			return diag.FromErr(err)
		}

		d.SetId("")
	*/
	return diags
}
