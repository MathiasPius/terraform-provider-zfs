package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// providerFactories are used to instantiate a provider during acceptance testing.
// The factory function will be invoked for every Terraform CLI command executed
// to create a provider server to which the CLI can reattach.
//
//lint:ignore U1000 this is used during acceptance testing. It's not unused.
var providerFactories = map[string]func() (*schema.Provider, error){
	"zfs": func() (*schema.Provider, error) {
		return New("dev")(), nil
	},
}

func TestProvider(t *testing.T) {
	if err := New("dev")().InternalValidate(); err != nil {
		t.Fatalf("err: %s", err)
	}
}

func testAccPreCheck(t *testing.T) {
	// You can add code here to run prior to any test case execution, for example assertions
	// about the appropriate environment variables being set are common to see in a pre-check
	// function.
	if err := os.Getenv("ZFS_PROVIDER_HOSTNAME"); err == "" {
		t.Fatal("ZFS_PROVIDER_HOSTNAME must be set for acceptance tests")
	}
	if err := os.Getenv("ZFS_PROVIDER_USERNAME"); err == "" {
		t.Fatal("ZFS_PROVIDER_HOSTNAME must be set for acceptance tests")
	}
}
