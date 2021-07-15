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
	creation   string
	used       string
	available  string
	referenced string
	mounted    string
	mountpoint string
}

func updateOption(ssh *easyssh.MakeConfig, datasetName string, option string, value string) (string, error) {
	log.Printf("[DEBUG] changing zfs option %s for %s to %s", option, datasetName, value)
	cmd := fmt.Sprintf("sudo zfs set %s=%s %s", option, value, datasetName)
	return callSshCommand(ssh, cmd)
}

func describeDataset(ssh *easyssh.MakeConfig, datasetName string) (*Dataset, error) {
	cmd := fmt.Sprintf("sudo zfs get -H all %s", datasetName)
	stdout, err := callSshCommand(ssh, cmd)

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

		log.Printf("[DEBUG] CSV line: %s", line)

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
		default:
			log.Printf("[DEBUG] ignoring attribute: %s (%s)", line[1], line[2])
		}
	}

	return &dataset, nil
}

type CreateDataset struct {
	name       string
	mountpoint string
}

func createDataset(ssh *easyssh.MakeConfig, dataset *CreateDataset) (*Dataset, error) {

	options := make(map[string]string, 0)
	if dataset.mountpoint != "" {
		options["mountpoint"] = dataset.mountpoint
	}

	serialized_options := ""
	for k, v := range options {
		serialized_options = fmt.Sprintf(" -o %s=%s", k, v)
	}

	cmd := fmt.Sprintf("sudo zfs create %s %s", serialized_options, dataset.name)
	_, err := callSshCommand(ssh, cmd)

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
