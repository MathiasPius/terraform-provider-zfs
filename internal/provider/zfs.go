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

type Pool struct {
	guid     string
	size     string
	capacity string
}

func describePool(config *Config, poolname string) (*Pool, error) {
	stdout, err := callSshCommand(config, "zpool get -H all %s", poolname)

	if err != nil {
		return nil, err
	}

	reader := csv.NewReader(strings.NewReader(stdout))
	reader.Comma = '\t'

	pool := Pool{}
	for {
		line, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		switch line[1] {
		case "size":
			pool.size = line[2]
		case "capacity":
			pool.capacity = line[2]
		case "guid":
			pool.guid = line[2]
		default:
			// do nothing
		}
	}

	return &pool, nil
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
	/*
		if pool.mountpoint != "" {
			options["mountpoint"] = dataset.mountpoint
		}
	*/

	serialized_options := ""
	for k, v := range options {
		serialized_options = fmt.Sprintf(" -o %s=%s", k, v)
	}

	_, err := callSshCommand(config, "zpool create %s %s %s", serialized_options, pool.name, pool.vdevs)

	if err != nil {
		// We might have an error, but it's possible that the dataset was still created
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
