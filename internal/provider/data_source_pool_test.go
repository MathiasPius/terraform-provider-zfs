package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourcePool(t *testing.T) {
	t.Skip("data source not yet implemented, remove this once you add your own code")

	resource.UnitTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourcePool,
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(
						"data.zfs_pool.foo", "name", regexp.MustCompile("^ba")),
				),
			},
		},
	})
}

const testAccDataSourcePool = `
data "zfs_pool" "foo" {
  name = "bar"
}
`
