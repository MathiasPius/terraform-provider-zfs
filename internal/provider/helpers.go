package provider

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

func callSshCommand(config *Config, cmd string, args ...interface{}) (string, error) {
	cmd = fmt.Sprintf(cmd, args...)
	log.Printf("[DEBUG] ssh command: %s", cmd)
	stdout, stderr, done, err := config.ssh.Run(cmd, 60*time.Second)

	if stderr != "" {
		if strings.Contains(stderr, "dataset does not exist") {
			return "", &DatasetError{errmsg: "dataset does not exist"}
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
	output, err := callSshCommand(config, "sudo stat -c '%%U,%%G,%%u,%%g' '%s'", path)

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
