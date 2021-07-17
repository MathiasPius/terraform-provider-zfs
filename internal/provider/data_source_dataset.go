package provider

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/appleboy/easyssh-proxy"
)

func dataSourceDataset() *schema.Resource {
	return &schema.Resource{
		// This description is used by the documentation generator and the language server.
		Description: "Data about a specific dataset.",

		ReadContext: dataSourceDatasetRead,

		Schema: map[string]*schema.Schema{
			"name": {
				// This description is used by the documentation generator and the language server.
				Description: "Name of the zfs dataset.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"mountpoint": {
				// This description is used by the documentation generator and the language server.
				Description: "Mountpoint of the dataset.",
				Type:        schema.TypeSet,
				Computed:    true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"path": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"uid": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"gid": {
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func dataSourceDatasetRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	ssh := meta.(*easyssh.MakeConfig)

	datasetName := d.Get("name").(string)
	dataset, err := describeDataset(ssh, datasetName)

	if dataset == nil {
		log.Println("[DEBUG] zfs dataset does not exist!")
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

	mountpoint := []map[string]interface{}{make(map[string]interface{})}
	mountpoint[0]["path"] = dataset.mountpoint
	if dataset.mountpoint != "" && dataset.mountpoint != "legacy" && dataset.mountpoint != "none" {
		// If mountpoint is specified, check the owner of the path
		cmd := fmt.Sprintf("sudo stat -c '%%U,%%G' '%s'", dataset.mountpoint)
		log.Printf("[DEBUG] stat command: %s", cmd)
		stdout, stderr, done, err := ssh.Run(cmd, 60*time.Second)

		if err != nil {
			return diag.FromErr(err)
		}

		if stderr != "" {
			return diag.FromErr(fmt.Errorf("stdout error: %s", stderr))
		}

		if !done {
			return diag.Errorf("command timed out")
		}

		reader := csv.NewReader(strings.NewReader(stdout))
		line, err := reader.Read()
		if err != nil {
			diag.FromErr(err)
		}

		mountpoint[0]["uid"] = line[0]
		mountpoint[0]["gid"] = line[1]
	}

	d.Set("mountpoint", mountpoint)
	d.SetId(dataset.guid)

	return diags
}
