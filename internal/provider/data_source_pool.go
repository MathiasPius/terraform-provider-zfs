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

func dataSourcePool() *schema.Resource {
	return &schema.Resource{
		// This description is used by the documentation generator and the language server.
		Description: "Sample data source in the Terraform provider scaffolding.",

		ReadContext: dataSourcePoolRead,

		Schema: map[string]*schema.Schema{
			"id": {
				// This description is used by the documentation generator and the language server.
				Description: "Name of the zpool.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"size": {
				Description: "Size of the pool.",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"capacity": {
				Description: "Capacity of the pool.",
				Type:        schema.TypeString,
				Computed:    true,
			},
		},
	}
}

func dataSourcePoolRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	// use the meta value to retrieve your client from the provider configure method
	// client := meta.(*apiClient)
	var diags diag.Diagnostics

	ssh := meta.(*easyssh.MakeConfig)

	pool_name := d.Get("id").(string)

	stdout, stderr, done, err := ssh.Run(fmt.Sprintf("sudo zpool get -H size,capacity %s", pool_name), 60*time.Second)

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
	reader.Comma = '\t'

	for {
		line, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			diag.FromErr(err)
		}

		log.Printf("[DEBUG] CSV line: %s", line)

		if err := d.Set(line[1], line[2]); err != nil {
			return diag.FromErr(err)
		}
	}

	d.SetId(pool_name)

	return diags
}
