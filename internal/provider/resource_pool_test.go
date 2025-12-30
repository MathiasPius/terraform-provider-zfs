package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// TestPropertyModeSchema_Validation verifies that only valid values
// are accepted for property_mode.
func TestPropertyModeSchema_Validation(t *testing.T) {
	valid := []string{"defined", "native", "all"}
	invalid := []string{"", "foo", "DEFINED", "everything"}

	for _, v := range valid {
		diags := propertyModeSchema.ValidateDiagFunc(v, nil)
		if len(diags) != 0 {
			t.Fatalf("expected %q to be valid, got diagnostics: %#v", v, diags)
		}
	}

	for _, v := range invalid {
		diags := propertyModeSchema.ValidateDiagFunc(v, nil)
		if len(diags) == 0 {
			t.Fatalf("expected %q to be invalid", v)
		}
	}
}

// TestParsePropertyBlocks_Empty verifies empty input yields no properties.
func TestParsePropertyBlocks_Empty(t *testing.T) {
	props := parsePropertyBlocks([]interface{}{})
	if len(props) != 0 {
		t.Fatalf("expected empty property map, got %#v", props)
	}
}

// TestParsePropertyBlocks_Multiple verifies multiple properties
// are parsed correctly.
func TestParsePropertyBlocks_Multiple(t *testing.T) {
	props := parsePropertyBlocks([]interface{}{
		map[string]interface{}{"name": "compression", "value": "on"},
		map[string]interface{}{"name": "ashift", "value": "12"},
	})

	if props["compression"] != "on" {
		t.Fatalf("compression property incorrect: %#v", props)
	}
	if props["ashift"] != "12" {
		t.Fatalf("ashift property incorrect: %#v", props)
	}
}

// TestPopulateResourceDataPool_Basic verifies that devices, mirrors,
// properties, and ID are correctly written to state.
func TestPopulateResourceDataPool_Basic(t *testing.T) {
	rd := resourcePool().TestResourceData()

	pool := Pool{
		guid: "pool-guid-123",
		layout: PoolLayout{
			striped: []Device{
				{path: "/dev/sda"},
			},
			mirrors: []Mirror{
				{
					devices: []Device{
						{path: "/dev/sdb"},
						{path: "/dev/sdc"},
					},
				},
			},
		},
		properties: map[string]Property{},
	}

	diags := populateResourceDataPool(rd, pool)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}

	if rd.Id() != "pool-guid-123" {
		t.Fatalf("expected ID to be set to pool GUID")
	}

	if got := len(rd.Get("device").([]interface{})); got != 1 {
		t.Fatalf("expected 1 striped device, got %d", got)
	}

	if got := len(rd.Get("mirror").([]interface{})); got != 1 {
		t.Fatalf("expected 1 mirror, got %d", got)
	}

	//check that the stripped device is correct
	deviceBlock := rd.Get("device").([]interface{})[0].(map[string]interface{})
	if deviceBlock["path"] != "/dev/sda" {
		t.Fatalf("expected striped device path /dev/sda, got %q", deviceBlock["path"])
	}

	//check that the mirror devices are correct
	mirrorBlock := rd.Get("mirror").([]interface{})[0].(map[string]interface{})
	devices := mirrorBlock["device"].([]interface{})
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices in mirror, got %d", len(devices))
	}
	device1 := devices[0].(map[string]interface{})
	device2 := devices[1].(map[string]interface{})
	if device1["path"] != "/dev/sdb" {
		t.Fatalf("expected first mirror device path /dev/sdb, got %q", device1["path"])
	}
	if device2["path"] != "/dev/sdc" {
		t.Fatalf("expected second mirror device path /dev/sdc, got %q", device2["path"])
	}
}

// buildResourceDataPool builds an empty ResourceData for the pool resource
// and pre-sets attributes needed by updatePropertiesInState.
// This avoids needing a real provider configuration or ZFS backend.
func buildResourceDataPool(t *testing.T) *schema.ResourceData {
	t.Helper()

	res := resourcePool()
	rd := res.TestResourceData()

	// Minimal required attributes
	if err := rd.Set("name", "aquarium"); err != nil {
		t.Fatalf("failed to set name: %v", err)
	}

	// Start with empty property set
	if err := rd.Set("property", schema.NewSet(propertyHash, []interface{}{})); err != nil {
		t.Fatalf("failed to set property: %v", err)
	}

	return rd
}

// propertyHash is the hash function used by the Terraform Set
// for the `property` block. Properties are uniquely identified
// by their name/value pair.
func propertyHash(v interface{}) int {
	m := v.(map[string]interface{})
	return schema.HashString(m["name"].(string) + "=" + m["value"].(string))
}

// TestUpdatePropertiesInState_DefaultsToDefinedWhenUnset verifies that
// when `property_mode` is not explicitly set, the default behavior
// matches "defined" mode.
//
// In this mode, only properties explicitly declared in configuration
// should be persisted in state. Since no properties were defined,
// no synthesized property blocks should be written.
func TestUpdatePropertiesInState_DefaultsToDefinedWhenUnset(t *testing.T) {
	rd := buildResourceDataPool(t)

	props := map[string]Property{
		"compression": {source: SourceLocal, value: "on", rawValue: "on"},
		"mountpoint":  {source: SourceInherited, value: "/aquarium", rawValue: "/aquarium"},
		"ashift":      {source: SourceLocal, value: "12", rawValue: "12"},
	}

	if err := updatePropertiesInState(rd, props, []string{}); err != nil {
		t.Fatalf("updatePropertiesInState returned error: %v", err)
	}

	got := rd.Get("property").(*schema.Set).List()
	if len(got) != 0 {
		t.Fatalf("expected 0 synthesized property blocks, got %d: %#v", len(got), got)
	}
}

// TestUpdatePropertiesInState_ModeDefinedKeepsOnlyExplicit verifies that
// in `property_mode = "defined"`, Terraform state retains only the
// property blocks explicitly declared by the user, even if additional
// properties are present on the ZFS pool.
//
// This ensures Terraform does not silently manage or persist properties
// outside of user intent.
func TestUpdatePropertiesInState_ModeDefinedKeepsOnlyExplicit(t *testing.T) {
	rd := buildResourceDataPool(t)

	if err := rd.Set("property_mode", "defined"); err != nil {
		t.Fatalf("failed to set property_mode: %v", err)
	}

	if err := rd.Set("property", schema.NewSet(propertyHash, []interface{}{
		map[string]interface{}{"name": "compression", "value": "on"},
	})); err != nil {
		t.Fatalf("failed to set property: %v", err)
	}

	props := map[string]Property{
		"compression": {source: SourceLocal, value: "on", rawValue: "on"},
		"mountpoint":  {source: SourceInherited, value: "/aquarium", rawValue: "/aquarium"},
		"ashift":      {source: SourceLocal, value: "12", rawValue: "12"},
	}

	if err := updatePropertiesInState(rd, props, []string{}); err != nil {
		t.Fatalf("updatePropertiesInState returned error: %v", err)
	}

	got := rd.Get("property").(*schema.Set).List()
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 property block, got %d: %#v", len(got), got)
	}

	block := got[0].(map[string]interface{})
	if block["name"] != "compression" || block["value"] != "on" {
		t.Fatalf("unexpected block: %#v", block)
	}
}

// TestUpdatePropertiesInState_ModeNativeSynthesizesOverriddenNativeOnly
// verifies that in `property_mode = "native"`, only overridden *native*
// ZFS properties are synthesized into Terraform state.
//
// User properties (e.g., userquota:*) and inherited properties should
// be ignored in this mode.
func TestUpdatePropertiesInState_ModeNativeSynthesizesOverriddenNativeOnly(t *testing.T) {
	rd := buildResourceDataPool(t)

	if err := rd.Set("property_mode", "native"); err != nil {
		t.Fatalf("failed to set property_mode: %v", err)
	}

	props := map[string]Property{
		"compression":        {source: SourceLocal, value: "on", rawValue: "on"},
		"userquota:someuser": {source: SourceLocal, value: "4G", rawValue: "4G"},
		"mountpoint":         {source: SourceInherited, value: "/aquarium", rawValue: "/aquarium"},
		"ashift":             {source: SourceLocal, value: "12", rawValue: "12"},
	}

	if err := updatePropertiesInState(rd, props, []string{}); err != nil {
		t.Fatalf("updatePropertiesInState returned error: %v", err)
	}

	got := rd.Get("property").(*schema.Set).List()
	if len(got) != 1 {
		t.Fatalf("expected 1 synthesized property block, got %d: %#v", len(got), got)
	}

	block := got[0].(map[string]interface{})
	if block["name"] != "compression" {
		t.Fatalf("expected compression only, got: %#v", block)
	}
}

// TestUpdatePropertiesInState_ModeAllSynthesizesOverriddenNativeAndUser
// verifies that in `property_mode = "all"`, both overridden native
// properties and user properties are synthesized into Terraform state.
//
// Inherited properties are still excluded, as they do not represent
// explicit overrides.
func TestUpdatePropertiesInState_ModeAllSynthesizesOverriddenNativeAndUser(t *testing.T) {
	rd := buildResourceDataPool(t)

	if err := rd.Set("property_mode", "all"); err != nil {
		t.Fatalf("failed to set property_mode: %v", err)
	}

	props := map[string]Property{
		"compression":        {source: SourceLocal, value: "on", rawValue: "on"},
		"userquota:someuser": {source: SourceTemporary, value: "4G", rawValue: "4G"},
		"mountpoint":         {source: SourceInherited, value: "/aquarium", rawValue: "/aquarium"},
		"ashift":             {source: SourceLocal, value: "12", rawValue: "12"},
	}

	if err := updatePropertiesInState(rd, props, []string{}); err != nil {
		t.Fatalf("updatePropertiesInState returned error: %v", err)
	}

	got := rd.Get("property").(*schema.Set).List()
	if len(got) != 2 {
		t.Fatalf("expected 2 synthesized property blocks, got %d: %#v", len(got), got)
	}

	names := map[string]bool{}
	for _, b := range got {
		names[b.(map[string]interface{})["name"].(string)] = true
	}

	if !names["compression"] || !names["userquota:someuser"] {
		t.Fatalf("expected compression and userquota:someuser, got: %#v", names)
	}
}

// TestIsPoolProperty_RecognizesKnownAndFeatureProps verifies that
// isPoolProperty correctly identifies which properties belong to
// ZFS pools rather than datasets.
//
// This includes both standard pool properties and feature@ properties,
// while excluding dataset-only properties.
func TestIsPoolProperty_RecognizesKnownAndFeatureProps(t *testing.T) {
	for _, p := range []string{"ashift", "autoreplace", "autoexpand", "readonly"} {
		if !isPoolProperty(p) {
			t.Fatalf("expected isPoolProperty(%q) == true", p)
		}
	}

	if !isPoolProperty("feature@encryption") {
		t.Fatalf("expected isPoolProperty(feature@encryption) == true")
	}

	for _, p := range []string{"compression", "mountpoint", "quota"} {
		if isPoolProperty(p) {
			t.Fatalf("expected isPoolProperty(%q) == false", p)
		}
	}
}
