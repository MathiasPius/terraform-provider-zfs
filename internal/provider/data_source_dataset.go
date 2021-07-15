package provider

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
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
			"id": {
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

	dataset_name := d.Get("id").(string)

	cmd := fmt.Sprintf("sudo zfs get -H mountpoint %s", dataset_name)

	stdout, err := callSshCommand(ssh, cmd)

	if err != nil {
		return diag.FromErr(err)
	}

	reader := csv.NewReader(strings.NewReader(stdout))
	reader.Comma = '\t'

	mountpoint := []map[string]interface{}{make(map[string]interface{})}

	for {
		line, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			diag.FromErr(err)
		}

		log.Printf("[DEBUG] CSV line: %s", line)

		if line[1] == "mountpoint" {
			mountpoint[0]["path"] = line[2]
		}
	}

	if path, ok := mountpoint[0]["path"]; ok && path != "legacy" {
		// If mountpoint is specified, check the owner of the path
		cmd := fmt.Sprintf("sudo stat -c '%%U,%%G' '%s'", path)
		log.Printf("[DEBUG] stat command: %s", cmd)
		stdout, stderr, done, err := ssh.Run(cmd, 60*time.Second)

		if err != nil {
			return diag.FromErr(err)
		}

		if stderr != "" {
			return diag.FromErr(errors.New(fmt.Sprintf("stdout error: %s", stderr)))
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
	d.SetId(dataset_name)

	return diags
}
