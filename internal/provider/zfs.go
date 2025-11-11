package provider

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/alessio/shellescape"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

type PropertySource string

const (
	SourceLocal     PropertySource = "local"
	SourceDefault   PropertySource = "default"
	SourceInherited PropertySource = "inherited"
	SourceTemporary PropertySource = "temporary"
	SourceReceived  PropertySource = "received"
	SourceNone      PropertySource = "none"
)

type DatasetType string

const (
	//BookmarkType DatasetType = "bookmark"
	FilesystemType DatasetType = "filesystem"
	//SnapshotType DatasetType = "snapshot"
	VolumeType DatasetType = "volume"
)

func parsePropertySource(input string) (PropertySource, error) {
	// Inherited is actually represented as "inherited from ...", so we only look at the first word.
	switch parts := strings.SplitN(input, " ", 2); parts[0] {
	case string(SourceLocal):
		return SourceLocal, nil
	case string(SourceDefault):
		return SourceDefault, nil
	case string(SourceInherited):
		return SourceInherited, nil
	case string(SourceTemporary):
		return SourceTemporary, nil
	case string(SourceReceived):
		return SourceReceived, nil
	case "-":
		return SourceNone, nil
	default:
		return "", fmt.Errorf("unrecognized source %s", input)
	}
}

var poolProperties = []string{
	"allocated",
	"altroot",
	"ashift",
	"autoexpand",
	"autoreplace",
	"autotrim",
	"bootfs",
	"cachefile",
	"capacity",
	"checkpoint",
	"comment",
	"compatibility",
	"dedupratio",
	"delegation",
	"expandsize",
	"failmode",
	"fragmentation",
	"free",
	"freeing",
	"guid",
	"health",
	"leaked",
	"listsnapshots",
	"load_guid",
	"multihost",
	"readonly",
	"size",
	"version",
}

func isPoolProperty(property string) bool {
	for _, poolProperty := range poolProperties {
		if property == poolProperty {
			return true
		}
	}
	return strings.HasPrefix(property, "feature@")
}

type Property struct {
	source   PropertySource
	value    string
	rawValue string
}

func readSomeProperties(config *Config, baseCommand string, resourceName string, propertyName string, properties map[string]Property) error {
	// First read the regular (formatted) values + the sources.
	stdout, err := callSshCommand(config, "%s get -H -o property,source,value %s %s", baseCommand, propertyName, resourceName)
	if err != nil {
		return err
	}

	reader := csv.NewReader(strings.NewReader(stdout))
	reader.Comma = '\t'
	for {
		line, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		name := line[0]
		property := Property{}
		property.value = line[2]
		if source, err := parsePropertySource(line[1]); err == nil {
			property.source = source
		} else {
			return fmt.Errorf("Error in property %s: %s", name, err)
		}
		properties[name] = property
	}

	// Then read the properties again in -p(arsable) mode to get the raw values.
	stdout, err = callSshCommand(config, "%s get -Hp -o property,value %s %s", baseCommand, propertyName, resourceName)
	if err != nil {
		return err
	}

	reader = csv.NewReader(strings.NewReader(stdout))
	reader.Comma = '\t'
	for {
		line, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		name := line[0]
		property, ok := properties[name]
		if !ok {
			continue
		}
		property.rawValue = line[1]
		properties[name] = property
	}

	return nil
}

func readAllProperties(config *Config, baseCommand string, resourceName string, requiredProperties []string, properties map[string]Property) error {
	if err := readSomeProperties(config, baseCommand, resourceName, "all", properties); err != nil {
		return err
	}
	// Most properties will have been fetched by querying 'all', but some are only returned when specifically asked for
	// (e.g. userquota@username), so check if any required properties are missing and fetch them now.
	missing := make([]string, 0)
	for _, property := range requiredProperties {
		if _, ok := properties[property]; !ok {
			missing = append(missing, property)
		}
	}
	if len(missing) > 0 {
		if err := readSomeProperties(config, baseCommand, resourceName, strings.Join(requiredProperties, ","), properties); err != nil {
			return err
		}
	}
	return nil
}

func readDatasetProperties(config *Config, datasetName string, requiredProperties []string, properties map[string]Property) error {
	requiredDatasetProperties := make([]string, 0)
	for _, property := range requiredProperties {
		if !isPoolProperty(property) {
			requiredDatasetProperties = append(requiredDatasetProperties, property)
		}
	}
	return readAllProperties(config, "zfs", datasetName, requiredDatasetProperties, properties)
}

func readPoolProperties(config *Config, poolName string, requiredProperties []string, properties map[string]Property) error {
	requiredPoolProperties := make([]string, 0)
	for _, property := range requiredProperties {
		if isPoolProperty(property) {
			requiredPoolProperties = append(requiredPoolProperties, property)
		}
	}
	return readAllProperties(config, "zpool", poolName, requiredPoolProperties, properties)
}

func updateCalculatedPropertiesInState(d *schema.ResourceData, properties map[string]Property) error {
	if err := d.Set("properties", flattenProperties(properties)); err != nil {
		return err
	}
	return d.Set("raw_properties", flattenRawProperties(properties))
}

func updatePropertiesInState(d *schema.ResourceData, properties map[string]Property, ignoredProperties []string) error {
	if err := updateCalculatedPropertiesInState(d, properties); err != nil {
		return err
	}

	defined := make(map[string]string)
	for _, property := range d.Get("property").(*schema.Set).List() {
		property := property.(map[string]interface{})
		defined[property["name"].(string)] = property["value"].(string)
	}

	ignored := make(map[string]bool)
	for _, name := range ignoredProperties {
		ignored[name] = true
	}

	mode := d.Get("property_mode").(string)
	blocks := make([]interface{}, 0)
	for name, property := range properties {
		if ignored[name] {
			continue
		}
		if _, ok := defined[name]; !ok {
			switch mode {
			case "defined":
				continue
			case "native":
				if strings.Contains(name, ":") {
					continue
				}
				fallthrough // native is just all with a filter for user properties, so fallthrough now that the filter has been applied.
			case "all":
				if property.source != SourceLocal && property.source != SourceTemporary {
					// Ignore properties that aren't in some way overridden on the resource.
					continue
				}
				if isPoolProperty(name) {
					// Pool properties cannot be reset to a default value (as there is no `zfs inherit` equivalent for zpool),
					// so there is no point tracking these unless they are defined in the property (in which case we have a
					// target value).
					continue
				}
			default:
				return fmt.Errorf("invalid value %s for property_mode", mode)
			}
		}
		block := make(map[string]interface{}, 0)
		block["name"] = name
		block["value"] = property.value
		if defined[name] == property.rawValue {
			block["value"] = property.rawValue
		}
		blocks = append(blocks, block)
	}
	return d.Set("property", blocks)
}

func getResetCommand(property string) (string, bool) {
	if isPoolProperty(property) {
		return "zpool properties cannot be reset back to a default value", false
	}
	if strings.Contains(property, "quota@") {
		return fmt.Sprintf("zfs set %s=none", shellescape.Quote(property)), true
	}
	return fmt.Sprintf("zfs inherit -S %s", shellescape.Quote(property)), true
}

func applyPropertyDiff(
	config *Config,
	d *schema.ResourceData,
	targetName string,
	actualProperties map[string]Property,
	overrideProperties map[string]string,
) error {
	oldProperties_, newProperties_ := d.GetChange("property")
	oldProperties := oldProperties_.(*schema.Set)
	newProperties := newProperties_.(*schema.Set)

	// Ensure overridden properties haven't been set by the user, and inject them in the list
	for name, value := range overrideProperties {
		for _, property := range newProperties.List() {
			if property.(map[string]interface{})["name"] == name {
				return fmt.Errorf("don't set '%s' as a property block, use the dedicated attribute instead", name)
			}
		}
		property := make(map[string]interface{})
		property["name"] = name
		property["value"] = value
		newProperties.Add(property)
	}

	// Unset (inherit) all properties that are no longer defined.
	removedProperties := parsePropertyBlocks(oldProperties.Difference(newProperties).List())
	log.Printf("[DEBUG] removed properties: %s", removedProperties)
	for property := range removedProperties {
		if result, ok := getResetCommand(property); ok {
			if _, err := callSshCommand(config, "%s %s", result, targetName); err != nil {
				return err
			}
		} else {
			log.Printf("%s, leaving %s at whatever value it's currently at", result, property)
		}
	}

	// Update properties which don't match the desired state.
	desiredProperties := parsePropertyBlocks(newProperties.List())
	log.Printf("[DEBUG] desired properties: %s", desiredProperties)
	log.Printf("[DEBUG] actual properties: %s", actualProperties)
	for name, value := range desiredProperties {
		if value != actualProperties[name].value {
			baseCommand := "zfs"
			if isPoolProperty(name) {
				baseCommand = "zpool"
			}
			if _, err := callSshCommand(config, "%s set %s=%s %s", baseCommand, shellescape.Quote(name), shellescape.Quote(value), targetName); err != nil {
				return err
			}
		}
	}

	return nil
}

type Dataset struct {
	dsType     DatasetType
	guid       string
	creation   string
	used       string
	available  string
	referenced string
	mounted    string
	mountpoint string
	volsize    string
	properties map[string]Property
}

func getZfsResourceNameByGuid(config *Config, resource_type string, guid string) (*string, error) {
	stdout, err := callSshCommand(config, "%s list -H -o name,guid", resource_type)

	if err != nil {
		return nil, err
	}

	reader := csv.NewReader(strings.NewReader(stdout))
	reader.Comma = '\t'

	for {
		line, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		if line[1] == guid {
			log.Printf("[DEBUG] found resource by guid: %s", line[0])
			return &line[0], nil
		}
	}

	return nil, fmt.Errorf("no resource found with guid %s", guid)
}

func getDatasetNameByGuid(config *Config, guid string) (*string, error) {
	return getZfsResourceNameByGuid(config, "zfs", guid)
}

func getPoolNameByGuid(config *Config, guid string) (*string, error) {
	return getZfsResourceNameByGuid(config, "zpool", guid)
}

func describeDataset(config *Config, datasetName string, requiredProperties []string) (*Dataset, error) {
	properties := make(map[string]Property, 0)
	if err := readDatasetProperties(config, datasetName, requiredProperties, properties); err != nil {
		return nil, err
	}

	dataset := Dataset{}
	dataset.properties = properties
	dataset.creation = properties["creation"].value
	dataset.used = properties["used"].value
	dataset.available = properties["available"].value
	dataset.referenced = properties["referenced"].value
	dataset.mounted = properties["mounted"].value
	dataset.mountpoint = properties["mountpoint"].value
	dataset.volsize = properties["volsize"].rawValue
	dataset.guid = properties["guid"].value

	switch properties["type"].value {
	case "filesystem":
		dataset.dsType = FilesystemType
	case "volume":
		dataset.dsType = VolumeType
	default:
		return nil, fmt.Errorf("Unsupported zfs dataset type %s with guid %s", properties["type"].value, properties["guid"].value)
	}

	return &dataset, nil
}

type Device struct {
	path string
}

type Mirror struct {
	devices []Device
}

type Pool struct {
	guid       string
	properties map[string]Property
	layout     PoolLayout
}

type PoolLayout struct {
	mirrors []Mirror
	striped []Device
}

func readPoolLayout(config *Config, poolName string) (*PoolLayout, error) {
	log.Printf("[DEBUG] reading zpool layout for %s", poolName)
	stdout, err := callSshCommand(config, "zpool list -HPv %s", poolName)

	if err != nil {
		return nil, err
	}

	reader := csv.NewReader(strings.NewReader(stdout))
	reader.Comma = '\t'

	// First line of zpool list output is the pool name/statistics themselves,
	// so we skip this line, of course making sure that the read itself works.
	line, err := reader.Read()
	if err == io.EOF {
		return nil, &PoolError{errmsg: "failed to read pool layout"}
	} else if err != nil {
		return nil, err
	}

	log.Printf("[DEBUG] parsing zpool layout for %s", line)

	layout := PoolLayout{
		mirrors: make([]Mirror, 0),
		striped: make([]Device, 0),
	}

	for {
		line, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		// All vdevs prefixed with "mirror" indicate the start of a mirrored vdev definition.
		// mirror* is also a reserved name so we know that if it starts with mirror, it is a mirror.
		// This is further ensured because we use the -P flag (use full path) with the zpool list
		// command, meaning all divide vdevs should start with a a forward slash.
		if strings.HasPrefix(line[1], "mirror") {
			layout.mirrors = append(layout.mirrors, Mirror{
				devices: make([]Device, 0),
			})
		} else {

			// If no mirror vdev has been instantiated, this is just a plain striped vdev.
			if len(layout.mirrors) == 0 {
				layout.striped = append(layout.striped, Device{
					path: line[1],
				})
			} else {
				// Otherwise, this vdev belongs to the last defined mirror.
				mirror := &layout.mirrors[len(layout.mirrors)-1]
				mirror.devices = append(mirror.devices, Device{path: line[1]})
			}
		}
	}

	log.Printf("[DEBUG] pool layout: %s", layout)

	return &layout, nil
}

func describePool(config *Config, poolName string, requiredProperties []string) (*Pool, error) {
	layout, err := readPoolLayout(config, poolName)
	if err != nil {
		return nil, err
	}

	properties := make(map[string]Property, 0)
	if err := readDatasetProperties(config, poolName, requiredProperties, properties); err != nil {
		return nil, err
	}
	if err := readPoolProperties(config, poolName, requiredProperties, properties); err != nil {
		return nil, err
	}

	return &Pool{
		guid:       properties["guid"].value,
		properties: properties,
		layout:     *layout,
	}, nil
}

type CreateDataset struct {
	dsType     DatasetType
	name       string
	mountpoint string
	volsize    string
	sparse     bool
	properties map[string]string
}

func createDataset(config *Config, dataset *CreateDataset) (*Dataset, error) {
	properties := dataset.properties
	serialized_options := ""

	if dataset.dsType == FilesystemType {
		if dataset.mountpoint != "" {
			properties["mountpoint"] = dataset.mountpoint
		}
	} else if dataset.dsType == VolumeType {
		if dataset.sparse {
			serialized_options += " -s"
		}
		serialized_options += fmt.Sprintf(" -V %s", dataset.volsize)
	}

	for property, value := range properties {
		serialized_options += fmt.Sprintf(" -o %s=%s", shellescape.Quote(property), shellescape.Quote(value))
	}

	_, err := callSshCommand(config, "zfs create %s %s", serialized_options, dataset.name)

	if err != nil {
		// We might have an error, but it's possible that the dataset was still created
		fetch_dataset, fetcherr := describeDataset(config, dataset.name, mapKeys(properties))

		// This is really dumb, but return both?
		if fetcherr != nil {
			return fetch_dataset, err
		}

		return nil, err
	}

	fetch_dataset, fetcherr := describeDataset(config, dataset.name, mapKeys(properties))
	return fetch_dataset, fetcherr
}

func destroyDataset(config *Config, datasetName string) error {
	_, err := callSshCommand(config, "zfs destroy -r %s", datasetName)
	return err
}

func renameDataset(config *Config, oldName string, newName string) error {
	_, err := callSshCommand(config, "zfs rename %s %s", oldName, newName)
	return err
}

type CreatePool struct {
	name       string
	vdevs      string
	properties map[string]string
}

func createPool(config *Config, pool *CreatePool) (*Pool, error) {
	serialized_options := ""
	for property, value := range pool.properties {
		if isPoolProperty(property) {
			serialized_options += fmt.Sprintf(" -o %s=%s", shellescape.Quote(property), shellescape.Quote(value))
		} else {
			serialized_options += fmt.Sprintf(" -O %s=%s", shellescape.Quote(property), shellescape.Quote(value))
		}
	}

	_, err := callSshCommand(config, "zpool create %s %s %s", serialized_options, pool.name, pool.vdevs)

	if err != nil {
		// We might have an error, but it's possible that the pool was still created
		fetch_pool, fetcherr := describePool(config, pool.name, mapKeys(pool.properties))

		// This is really dumb, but return both?
		if fetcherr != nil {
			return fetch_pool, err
		}

		return nil, err
	}

	fetch_pool, fetcherr := describePool(config, pool.name, mapKeys(pool.properties))
	return fetch_pool, fetcherr
}

func renamePool(config *Config, oldName string, newName string) error {
	_, err := callSshCommand(config, "zpool export %s", oldName)
	if err != nil {
		return err
	}

	_, err = callSshCommand(config, "zpool import %s %s", oldName, newName)

	return err
}

func destroyPool(config *Config, poolName string) error {
	_, err := callSshCommand(config, "zpool destroy %s", poolName)
	return err
}

func flattenProperties(properties map[string]Property) map[string]interface{} {
	out := make(map[string]interface{})
	for name, property := range properties {
		out[name] = make(map[string]interface{})
		out[name] = property.value
	}

	return out
}

func flattenRawProperties(properties map[string]Property) map[string]interface{} {
	out := make(map[string]interface{})
	for name, property := range properties {
		out[name] = make(map[string]interface{})
		out[name] = property.rawValue
	}

	return out
}

func flattenMirror(mirror Mirror) map[string]interface{} {
	out := make(map[string]interface{})
	devices := make([]map[string]interface{}, len(mirror.devices))
	for device_id, device := range mirror.devices {
		device := flattenDevice(device)
		devices[device_id] = device
	}
	out["device"] = devices

	return out
}

func flattenDevice(device Device) map[string]interface{} {
	out := make(map[string]interface{})
	out["path"] = device.path

	return out
}
