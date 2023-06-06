package provider

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
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

var propertySchema = schema.Schema{
	Description: "Propert(y/ies) to set",
	Type:        schema.TypeSet,
	Optional:    true,
	Elem: &schema.Resource{
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
	},
}

var propertyModeSchema = schema.Schema{
	Description: `
		Which properties to manage.

		"defined" means only manage the properties explicitly defined in the resource. This is the default.

		"native" means manage all native zfs properties, but leave user properties alone (see man zfsprops for more info
		about these types of properties). This means all properties that aren't defined in the terraform resource but that
		are explicitly overriden on the zfs resource will be set back to inherit from their parent/the default.

		"all" is like "native", but also includes user properties. Be careful when removing/altering properties you don't
		recognize as some tools might use user properties to track information important for that tool to work properly
		with a given resource.

		Note that some properties don't have a default that they can be compared/reset to (notably most of the zpool
		properties). These properties will only ever be managed when explicitly defined, and will be left as they are when
		they stop being defined.
	`,
	Type:             schema.TypeString,
	Default:          "defined",
	Optional:         true,
	ValidateDiagFunc: validation.ToDiagFunc(validation.StringInSlice([]string{"defined", "native", "all"}, false)),
}

var propertiesSchema = schema.Schema{
	Description: "Formatted versions of all zfs properties.",
	Type:        schema.TypeMap,
	Computed:    true,
	Elem:        schema.TypeString,
}

var rawPropertiesSchema = schema.Schema{
	Description: "Parseable versions of all zfs properties.",
	Type:        schema.TypeMap,
	Computed:    true,
	Elem:        schema.TypeString,
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
			"property":       &propertySchema,
			"property_mode":  &propertyModeSchema,
			"properties":     &propertiesSchema,
			"raw_properties": &rawPropertiesSchema,
		},
	}
}

func resourcePoolCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	poolName := d.Get("name").(string)

	config := meta.(*Config)

	pool, err := describePool(config, poolName, getPropertyNames(d))
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

	vdev_spec, err := parseVdevSpecification(d.Get("mirror"), d.Get("device"))
	if err != nil {
		log.Printf("[DEBUG] failed to parse vdev specification")
		return diag.FromErr(err)
	}

	properties := parsePropertyBlocks(d.Get("property").(*schema.Set).List())

	pool, err = createPool(config, &CreatePool{
		name:       poolName,
		vdevs:      vdev_spec,
		properties: properties,
	})

	if err != nil {
		return diag.FromErr(err)
	}

	// We're setting the ID here because the dataset DOES exist, even if the mountpoint
	// is not properly configured!
	log.Printf("[DEBUG] committing guid: %s", pool.guid)
	d.SetId(pool.guid)

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

	pool, err := describePool(config, poolName, getPropertyNames(d))
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

	if err := updatePropertiesInState(d, pool.properties, []string{}); err != nil {
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
	if poolName != *old_name {
		if err := renamePool(config, *old_name, poolName); err != nil {
			return diag.FromErr(err)
		}
	}

	pool, err := describePool(config, poolName, getPropertyNames(d))
	if err != nil {
		return diag.FromErr(err)
	}

	err = applyPropertyDiff(config, d, poolName, pool.properties, make(map[string]string))
	if err != nil {
		return diag.FromErr(err)
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
