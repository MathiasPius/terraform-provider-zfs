package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccResourceDataset(t *testing.T) {
	t.Skip("resource not yet implemented, remove this once you add your own code")

	resource.UnitTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccresourceDataset,
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(
						"zfs_dataset.foo", "sample_attribute", regexp.MustCompile("^ba")),
				),
			},
		},
	})
}

const testAccresourceDataset = `
resource "zfs_dataset" "foo" {
  sample_attribute = "bar"
}
`
