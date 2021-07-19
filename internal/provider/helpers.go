package provider

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/appleboy/easyssh-proxy"
)

func callSshCommand(ssh *easyssh.MakeConfig, cmd string, args ...interface{}) (string, error) {
	cmd = fmt.Sprintf(cmd, args...)
	log.Printf("[DEBUG] ssh command: %s", cmd)
	stdout, stderr, done, err := ssh.Run(cmd, 60*time.Second)

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

	return stdout, nil
}

type Ownership struct {
	userName  string
	groupName string
	uid       string
	gid       string
}

func getFileOwnership(ssh *easyssh.MakeConfig, path string) (*Ownership, error) {
	output, err := callSshCommand(ssh, "sudo stat -c '%%U,%%G,%%u,%%g' '%s'", path)

	if err != nil {
		return nil, err
	}

	values := strings.Split(strings.TrimSuffix(output, "\n"), ",")

	return &Ownership{
		userName:  values[0],
		groupName: values[1],
		uid:       values[2],
		gid:       values[3],
	}, nil
}
