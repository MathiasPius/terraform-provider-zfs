package provider

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"strings"
)

type Dataset struct {
	guid       string
	creation   string
	used       string
	available  string
	referenced string
	mounted    string
	mountpoint string
}

func updateOption(config *Config, datasetName string, option string, value string) (string, error) {
	log.Printf("[DEBUG] changing zfs option %s for %s to %s", option, datasetName, value)
	return callSshCommand(config, "zfs set %s=%s %s", option, value, datasetName)
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

func describeDataset(config *Config, datasetName string) (*Dataset, error) {
	stdout, err := callSshCommand(config, "zfs get -H all %s", datasetName)

	if err != nil {
		return nil, err
	}

	reader := csv.NewReader(strings.NewReader(stdout))
	reader.Comma = '\t'

	dataset := Dataset{}
	for {
		line, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		switch line[1] {
		case "creation":
			dataset.creation = line[2]
		case "used":
			dataset.used = line[2]
		case "available":
			dataset.available = line[2]
		case "referenced":
			dataset.referenced = line[2]
		case "mounted":
			dataset.mounted = line[2]
		case "mountpoint":
			dataset.mountpoint = line[2]
		case "guid":
			dataset.guid = line[2]
		default:
			// do nothing
		}
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
	properties map[string]string
	layout     PoolLayout
}

type PoolLayout struct {
	mirrors []Mirror
	striped []Device
}

func readPoolProperties(config *Config, poolName string) (map[string]string, error) {
	stdout, err := callSshCommand(config, "zpool get -H all %s", poolName)

	if err != nil {
		return nil, err
	}

	reader := csv.NewReader(strings.NewReader(stdout))
	reader.Comma = '\t'

	log.Print("[DEBUG] parsing zpool properties")

	properties := make(map[string]string)

	for {
		line, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		properties[line[1]] = line[2]
	}

	log.Printf("[DEBUG] %s properties: %s", poolName, properties)

	return properties, nil
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
		// command, meaning all devide vdevs should start with a a forward slash.
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

func describePool(config *Config, poolName string) (*Pool, error) {
	properties, err := readPoolProperties(config, poolName)
	if err != nil {
		return nil, err
	}

	layout, err := readPoolLayout(config, poolName)
	if err != nil {
		return nil, err
	}

	return &Pool{
		guid:       properties["guid"],
		properties: properties,
		layout:     *layout,
	}, nil
}

type CreateDataset struct {
	name       string
	mountpoint string
}

func createDataset(config *Config, dataset *CreateDataset) (*Dataset, error) {

	options := make(map[string]string)
	if dataset.mountpoint != "" {
		options["mountpoint"] = dataset.mountpoint
	}

	serialized_options := ""
	for k, v := range options {
		serialized_options = fmt.Sprintf(" -o %s=%s", k, v)
	}

	_, err := callSshCommand(config, "zfs create %s %s", serialized_options, dataset.name)

	if err != nil {
		// We might have an error, but it's possible that the dataset was still created
		fetch_dataset, fetcherr := describeDataset(config, dataset.name)

		// This is really dumb, but return both?
		if fetcherr != nil {
			return fetch_dataset, err
		}

		return nil, err
	}

	fetch_dataset, fetcherr := describeDataset(config, dataset.name)
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
	name  string
	vdevs string
}

func createPool(config *Config, pool *CreatePool) (*Pool, error) {
	options := make(map[string]string)

	serialized_options := ""
	for k, v := range options {
		serialized_options = fmt.Sprintf(" -o %s=%s", k, v)
	}

	_, err := callSshCommand(config, "zpool create %s %s %s", serialized_options, pool.name, pool.vdevs)

	if err != nil {
		// We might have an error, but it's possible that the pool was still created
		fetch_pool, fetcherr := describePool(config, pool.name)

		// This is really dumb, but return both?
		if fetcherr != nil {
			return fetch_pool, err
		}

		return nil, err
	}

	fetch_pool, fetcherr := describePool(config, pool.name)
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

func flattenProperties(pool Pool) map[string]interface{} {
	out := make(map[string]interface{})
	for name, value := range pool.properties {
		out[name] = value
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
