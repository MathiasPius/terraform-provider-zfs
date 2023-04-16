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
			Type:        schema.TypeList,
			Required:    true,
			ForceNew:    true,
			Elem:        vdevSchema,
			MinItems:    2,
		},
	},
}

var propertySchema = &schema.Resource{
	Schema: map[string]*schema.Schema{
		"name": {
			Description: "The name of the property to configure",
			Type:        schema.TypeString,
			Required:    true,
		},
		"value": {
			Description: "Value of the property",
			Type:        schema.TypeString,
			Required:    true,
		},
	},
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
				Type:        schema.TypeList,
				Optional:    true,
				Elem:        mirrorSchema,
			},
			"device": {
				Description: "Defines a striped vdev",
				Type:        schema.TypeList,
				Optional:    true,
				AtLeastOneOf: []string{
					"device", "mirror",
				},
				ConflictsWith: []string{
					"mirror",
				},
				Elem: vdevSchema,
			},
			"property": {
				Description: "Propert(y/ies) to apply to the zpool",
				Type:        schema.TypeSet,
				Optional:    true,
				Elem:        propertySchema,
			},
			"properties": {
				Description: "All properties for a zpool.",
				Type:        schema.TypeMap,
				Computed:    true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
		},
	}
}

func resourcePoolCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
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

	return populateResourceDataPool(d, *pool)
}

func resourcePoolRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
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

	if err := d.Set("name", poolName); err != nil {
		diag.FromErr(err)
	}

	pool, err := describePool(config, poolName)
	if err != nil {
		return diag.FromErr(err)
	}

	return populateResourceDataPool(d, *pool)
}

func populateResourceDataPool(d *schema.ResourceData, pool Pool) diag.Diagnostics {
	var diags diag.Diagnostics

	devices := make([]map[string]interface{}, len(pool.layout.striped))
	for device_id, device := range pool.layout.striped {
		devices[device_id] = flattenDevice(device)
	}

	mirrors := make([]map[string]interface{}, len(pool.layout.mirrors))
	for mirror_id, mirror := range pool.layout.mirrors {
		mirrors[mirror_id] = flattenMirror(mirror)
	}

	if err := d.Set("device", devices); err != nil {
		return diag.FromErr(err)
	}

	if err := d.Set("mirror", mirrors); err != nil {
		return diag.FromErr(err)
	}

	if err := d.Set("properties", flattenProperties(pool)); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(pool.guid)
	return diags
}

func resourcePoolUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := meta.(*Config)
	old_name, err := getPoolNameByGuid(config, d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	poolName := d.Get("name").(string)
	// Rename the dataset
	if poolName != *old_name {
		if err := renamePool(config, *old_name, poolName); err != nil {
			return diag.FromErr(err)
		}
	}
	return resourcePoolRead(ctx, d, meta)
}

func resourcePoolDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	config := meta.(*Config)
	poolName := d.Get("name").(string)
	id := d.Get("id")

	log.Printf("[DEBUG] destroying pool: %s %d", poolName, id)
	if err := destroyPool(config, poolName); err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	return diags
}
