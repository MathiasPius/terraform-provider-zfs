package provider

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/appleboy/easyssh-proxy"
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

func updateOption(ssh *easyssh.MakeConfig, datasetName string, option string, value string) (string, error) {
	log.Printf("[DEBUG] changing zfs option %s for %s to %s", option, datasetName, value)
	return callSshCommand(ssh, "sudo zfs set %s=%s %s", option, value, datasetName)
}

func getDatasetNameByGuid(ssh *easyssh.MakeConfig, guid string) (*string, error) {
	stdout, err := callSshCommand(ssh, "sudo zfs list -H -o name,guid")

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
			log.Printf("[DEBUG] found dataset by guid: %s", line[0])
			return &line[0], nil
		}
	}

	return nil, fmt.Errorf("no dataset found with guid %s", guid)
}

func describeDataset(ssh *easyssh.MakeConfig, datasetName string) (*Dataset, error) {
	stdout, err := callSshCommand(ssh, "sudo zfs get -H all %s", datasetName)

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

type CreateDataset struct {
	name       string
	mountpoint string
}

func createDataset(ssh *easyssh.MakeConfig, dataset *CreateDataset) (*Dataset, error) {

	options := make(map[string]string)
	if dataset.mountpoint != "" {
		options["mountpoint"] = dataset.mountpoint
	}

	serialized_options := ""
	for k, v := range options {
		serialized_options = fmt.Sprintf(" -o %s=%s", k, v)
	}

	_, err := callSshCommand(ssh, "sudo zfs create %s %s", serialized_options, dataset.name)

	if err != nil {
		// We might have an error, but it's possible that the dataset was still created
		fetch_dataset, fetcherr := describeDataset(ssh, dataset.name)

		// This is really dumb, but return both?
		if fetcherr != nil {
			return fetch_dataset, err
		}

		return nil, err
	}

	fetch_dataset, fetcherr := describeDataset(ssh, dataset.name)
	return fetch_dataset, fetcherr
}

func destroyDataset(ssh *easyssh.MakeConfig, datasetName string) error {
	_, err := callSshCommand(ssh, "sudo zfs destroy -r %s", datasetName)

	if err != nil {
		return err
	}

	return nil
}

func renameDataset(ssh *easyssh.MakeConfig, oldName string, newName string) error {
	_, err := callSshCommand(ssh, "sudo zfs rename %s %s", oldName, newName)

	if err != nil {
		return err
	}

	return nil
}
