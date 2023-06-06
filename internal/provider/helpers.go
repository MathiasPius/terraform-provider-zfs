package provider

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func callSshCommand(config *Config, cmd string, args ...interface{}) (string, error) {
	cmd = fmt.Sprintf(cmd, args...)
	log.Printf("[DEBUG] ssh command: %s %s", config.command_prefix, cmd)
	stdout, stderr, done, err := config.ssh.Run(config.command_prefix+" "+cmd, 60*time.Second)

	if stderr != "" {
		if strings.Contains(stderr, "dataset does not exist") {
			return "", &DatasetError{errmsg: "dataset does not exist"}
		} else if strings.Contains(stderr, "no such pool") {
			return "", &PoolError{errmsg: "zpool does not exist"}
		} else {
			return "", &StderrError{stderr: stderr}
		}
	}

	if err != nil {
		return "", &SshConnectError{inner: err}
	}

	if !done {
		return "", &SshConnectError{inner: errors.New("command timed out")}
	}

	return strings.TrimSuffix(stdout, "\n"), nil
}

type Ownership struct {
	userName  string
	groupName string
	uid       int
	gid       int
}

func getFileOwnership(config *Config, path string) (*Ownership, error) {
	output, err := callSshCommand(config, "stat -c '%%U,%%G,%%u,%%g' '%s'", path)

	if err != nil {
		return nil, err
	}

	values := strings.Split(output, ",")

	uid, err := strconv.Atoi(values[2])
	if err != nil {
		return nil, err
	}

	gid, err := strconv.Atoi(values[3])
	if err != nil {
		return nil, err
	}

	return &Ownership{
		userName:  values[0],
		groupName: values[1],
		uid:       uid,
		gid:       gid,
	}, nil
}

func parseVdevSpecification(mirrors interface{}, devices interface{}) (string, error) {
	vdevs := ""
	if mirrors != nil {
		for _, mirror := range mirrors.([]interface{}) {
			devices := mirror.(map[string]interface{})["device"]
			vdevs = vdevs + " mirror"
			for _, device := range devices.([]interface{}) {
				path := device.(map[string]interface{})["path"].(string)
				vdevs = vdevs + " " + path
			}
		}
	}

	if devices != nil {
		for _, device := range devices.([]interface{}) {
			path := device.(map[string]interface{})["path"].(string)
			vdevs = vdevs + " " + path
		}
	}

	log.Printf("[DEBUG] vdev specification: %s", vdevs)
	return vdevs, nil
}

func parsePropertyBlocks(options []interface{}) map[string]string {
	properties := make(map[string]string)

	for _, option := range options {
		property := option.(map[string]interface{})
		properties[property["name"].(string)] = property["value"].(string)
	}

	return properties
}

func mapKeys[T interface{}](value map[string]T) []string {
	keys := make([]string, 0)
	for key, _ := range value {
		keys = append(keys, key)
	}
	return keys
}

func getPropertyNames(d *schema.ResourceData) []string {
	names := make([]string, 0)
	for _, property := range d.Get("property").(*schema.Set).List() {
		names = append(names, property.(map[string]interface{})["name"].(string))
	}
	return names
}
