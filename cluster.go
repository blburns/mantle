package pluton

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/coreos/mantle/platform"
	"github.com/coreos/mantle/util"
)

// Cluster represents an interface to test kubernetes clusters The creation is
// usually implemented by a function that builds the Cluster from a kola
// TestCluster from the 'spawn' subpackage. Tests may be aware of the
// implementor function since not all clusters are expected to have the same
// components nor properties.
type Cluster struct {
	Masters []platform.Machine
	Workers []platform.Machine

	m Manager
}

func NewCluster(m Manager, masters, workers []platform.Machine) *Cluster {
	return &Cluster{
		Masters: masters,
		Workers: workers,
		m:       m,
	}
}

// Kubectl will run kubectl from /home/core on the Master Machine
func (c *Cluster) Kubectl(cmd string) (string, error) {
	client, err := c.Masters[0].SSHClient()
	if err != nil {
		return "", err
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	var stdout = bytes.NewBuffer(nil)
	var stderr = bytes.NewBuffer(nil)
	session.Stderr = stderr
	session.Stdout = stdout

	err = session.Run("sudo ./kubectl --kubeconfig=/etc/kubernetes/kubeconfig " + cmd)
	if err != nil {
		return "", fmt.Errorf("kubectl:%s", stderr)
	}
	return stdout.String(), nil
}

// AddMasters creates new master nodes for a Cluster and blocks until ready.
func (c *Cluster) AddMasters(n int) error {
	nodes, err := c.m.AddMasters(n)
	if err != nil {
		return err
	}

	c.Masters = append(c.Masters, nodes...)

	if err := c.NodeCheck(12); err != nil {
		return err
	}
	return nil
}

// NodeCheck will parse kubectl output to ensure all nodes in Cluster are
// registered. Set retry for max amount of retries to attempt before erroring.
func (c *Cluster) NodeCheck(retryAttempts int) error {
	f := func() error {
		b, err := c.Masters[0].SSH("./kubectl get nodes")
		if err != nil {
			return err
		}

		// parse kubectl output for IPs
		addrMap := map[string]struct{}{}
		for _, line := range strings.Split(string(b), "\n")[1:] {
			addrMap[strings.SplitN(line, " ", 2)[0]] = struct{}{}
		}

		nodes := append(c.Workers, c.Masters...)

		if len(addrMap) != len(nodes) {
			return fmt.Errorf("cannot detect all nodes in kubectl output \n%v\n%v", addrMap, nodes)
		}
		for _, node := range nodes {
			if _, ok := addrMap[node.PrivateIP()]; !ok {
				return fmt.Errorf("node IP missing from kubectl get nodes")
			}
		}
		return nil
	}

	if err := util.Retry(retryAttempts, 10*time.Second, f); err != nil {
		return err
	}
	return nil
}
