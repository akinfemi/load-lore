package local

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/moby/moby/client"
)

// Labels on everything Loadlore creates. `loadl down` removes by these.
const (
	LabelOwner   = "io.loadlore"
	LabelRun     = "io.loadlore.run"
	LabelService = "io.loadlore.service"
	LabelReplica = "io.loadlore.replica"
	LabelRole    = "io.loadlore.role"
	LabelCache   = "io.loadlore.cache"
)

// dockerHost returns the Docker endpoint: DOCKER_HOST if set, otherwise
// the current docker context's endpoint (Colima and Docker Desktop users
// rely on contexts, which the Engine API client doesn't read).
func dockerHost(ctx context.Context) string {
	if h := os.Getenv("DOCKER_HOST"); h != "" {
		return h
	}
	out, err := exec.CommandContext(ctx, "docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// NewClient connects to the Docker Engine API.
func NewClient(ctx context.Context) (*client.Client, string, error) {
	host := dockerHost(ctx)
	opts := []client.Opt{client.FromEnv}
	if host != "" {
		opts = append(opts, client.WithHost(host))
	}
	c, err := client.New(opts...)
	if err != nil {
		return nil, "", fmt.Errorf("connecting to Docker: %w", err)
	}
	return c, host, nil
}

// docker runs the docker CLI with the same endpoint as the API client and
// returns its combined output.
func (d *Driver) docker(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = os.Environ()
	if d.host != "" {
		cmd.Env = append(cmd.Env, "DOCKER_HOST="+d.host)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("docker %s: %w\n%s", strings.Join(args, " "), err, tail(out, 2000))
	}
	return out, nil
}

// compose runs docker compose for this run's project.
func (d *Driver) compose(ctx context.Context, args ...string) ([]byte, error) {
	base := []string{"compose", "-p", d.project, "-f", d.composePath}
	return d.docker(ctx, append(base, args...)...)
}

func tail(b []byte, n int) string {
	if len(b) > n {
		b = b[len(b)-n:]
	}
	return string(b)
}

// containerIDs finds this run's containers for a compose service name.
func (d *Driver) containerIDs(ctx context.Context, service string) ([]string, error) {
	f := client.Filters{}.Add("label", LabelRun+"="+d.runID).Add("label", "com.docker.compose.service="+service)
	res, err := d.api.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: f})
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, c := range res.Items {
		ids = append(ids, c.ID)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no container for %s", service)
	}
	return ids, nil
}

// readFileFromContainer copies one file out of a container.
func (d *Driver) readFileFromContainer(ctx context.Context, id, path string) ([]byte, error) {
	res, err := d.api.CopyFromContainer(ctx, id, client.CopyFromContainerOptions{SourcePath: path})
	if err != nil {
		return nil, err
	}
	defer res.Content.Close()
	tr := tar.NewReader(res.Content)
	for {
		h, err := tr.Next()
		if err != nil {
			return nil, fmt.Errorf("reading %s from container: %w", path, err)
		}
		if h.Typeflag == tar.TypeReg {
			var b bytes.Buffer
			if _, err := io.Copy(&b, tr); err != nil {
				return nil, err
			}
			return b.Bytes(), nil
		}
	}
}

// dockerStdin runs the docker CLI with input on stdin.
func (d *Driver) dockerStdin(ctx context.Context, stdin string, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = os.Environ()
	if d.host != "" {
		cmd.Env = append(cmd.Env, "DOCKER_HOST="+d.host)
	}
	cmd.Stdin = strings.NewReader(stdin)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker %s: %w\n%s", strings.Join(args, " "), err, tail(out, 2000))
	}
	return nil
}
