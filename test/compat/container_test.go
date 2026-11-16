//go:build compat

package compat

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
)

// containerSpec describes one application container. Files are copied in before
// the first start so credentials exist before the application initializes.
type containerSpec struct {
	name  string
	image string
	port  int
	env   map[string]string
	files map[string]string
	args  []string
}

type container struct {
	ctr  testcontainers.Container
	port int
}

// startContainer starts the application with its port published on a random
// host port, and removes the container when the test ends. Logs are printed
// whenever the test failed; ARRCHITECT_COMPAT_KEEP=1 keeps the container.
func startContainer(t *testing.T, spec containerSpec) *container {
	t.Helper()
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        pinnedImage(t, spec.image),
		ExposedPorts: []string{strconv.Itoa(spec.port) + "/tcp"},
		Env:          spec.env,
		Cmd:          spec.args,
		Labels:       map[string]string{"arrchitect-compat": "true"},
	}
	for path, content := range spec.files {
		req.Files = append(req.Files, testcontainers.ContainerFile{
			Reader: strings.NewReader(content), ContainerFilePath: path, FileMode: 0o644,
		})
	}
	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req, Started: true,
	})
	if os.Getenv("ARRCHITECT_COMPAT_KEEP") != "1" {
		testcontainers.CleanupContainer(t, ctr)
	}
	require.NoError(t, err, "starting %s", spec.name)
	c := &container{ctr: ctr}
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("%s logs:\n%s", spec.name, tail(c.logs(t), 80))
		}
	})

	mapped, err := ctr.MappedPort(ctx, strconv.Itoa(spec.port)+"/tcp")
	require.NoError(t, err, "reading %s port", spec.name)
	c.port = int(mapped.Num())
	return c
}

// run runs a shell command inside the container and returns its output.
func (c *container) run(script string) (string, error) {
	code, out, err := c.ctr.Exec(context.Background(), []string{"sh", "-c", script}, tcexec.Multiplexed())
	if err != nil {
		return "", err
	}
	data, err := io.ReadAll(out)
	if err != nil {
		return "", err
	}
	output := strings.TrimSpace(string(data))
	if code != 0 {
		return output, fmt.Errorf("%q exited %d: %s", script, code, output)
	}
	return output, nil
}

// exec runs a shell command inside the container and fails the test if it fails.
func (c *container) exec(t *testing.T, script string) string {
	t.Helper()
	out, err := c.run(script)
	require.NoError(t, err)
	return out
}

func (c *container) logs(t *testing.T) string {
	t.Helper()
	r, err := c.ctr.Logs(context.Background())
	if err != nil {
		return err.Error()
	}
	defer func() { _ = r.Close() }()
	data, _ := io.ReadAll(r)
	return string(data)
}

// url is the application address as both the operator and other containers
// reach it: the host's outbound IP and the published port.
func (c *container) url() string {
	return fmt.Sprintf("http://%s:%d", hostIP, c.port)
}

func tail(s string, lines int) string {
	all := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	return strings.Join(all, "\n")
}
