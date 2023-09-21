package main

import (
	"context"
	"fmt"
	"net"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"

	"github.com/crossplane/crossplane-runtime/pkg/errors"

	pkgv1beta1 "github.com/crossplane/crossplane/apis/pkg/v1beta1"
)

// RuntimeDocker uses a Docker daemon to run a Function.
type RuntimeDocker struct {
	// Image to run
	Image string

	// Stop container when
	Stop bool
}

// GetRuntimeDocker extracts RuntimeDocker configuration from the supplied
// Function.
func GetRuntimeDocker(fn pkgv1beta1.Function) *RuntimeDocker {
	// TODO(negz): Support overriding with annotations.
	// TODO(negz): Pull package in case it has a different controller image?
	r := &RuntimeDocker{
		Image: fn.Spec.Package,
		Stop:  true,
	}
	return r
}

var _ Runtime = &RuntimeDocker{}

// Start a Function as a Docker container.
func (r *RuntimeDocker) Start(ctx context.Context) (RuntimeContext, error) {
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return RuntimeContext{}, errors.Wrap(err, "cannot create Docker client using environment variables")
	}

	out, err := c.ImagePull(ctx, r.Image, types.ImagePullOptions{})
	if err != nil {
		return RuntimeContext{}, errors.Wrapf(err, "cannot pull Docker image %q", r.Image)
	}
	defer out.Close() //nolint:errcheck // TODO(negz): Can this error?

	// Find a random, available port. There's a chance of a race here, where
	// something else binds to the port before we start our container.
	lis, err := net.Listen("tcp", ":0") //nolint:gosec // We're only doing this briefly to find a port.
	if err != nil {
		return RuntimeContext{}, errors.Wrap(err, "cannot get available TCP port")
	}
	addr := lis.Addr().String()
	_ = lis.Close()

	spec := fmt.Sprintf("%s:9443/tcp", addr)
	expose, bind, err := nat.ParsePortSpecs([]string{spec})
	if err != nil {
		return RuntimeContext{}, errors.Wrapf(err, "cannot parse Docker port spec %q", spec)
	}

	cfg := &container.Config{
		Image:        r.Image,
		Cmd:          []string{"--insecure"},
		ExposedPorts: expose,
	}
	hcfg := &container.HostConfig{
		PortBindings: bind,
	}

	// TODO(negz): Set a container name. Presumably unique across runs.
	rsp, err := c.ContainerCreate(ctx, cfg, hcfg, nil, nil, "")
	if err != nil {
		return RuntimeContext{}, errors.Wrap(err, "cannot create Docker container")
	}

	if err := c.ContainerStart(ctx, rsp.ID, types.ContainerStartOptions{}); err != nil {
		return RuntimeContext{}, errors.Wrap(err, "cannot start Docker container")
	}

	stop := func(_ context.Context) error {
		// TODO(negz): Maybe log to stderr that we're leaving the container running?
		return nil
	}
	if r.Stop {
		stop = func(ctx context.Context) error {
			err := c.ContainerStop(ctx, rsp.ID, container.StopOptions{})
			return errors.Wrap(err, "cannot stop Docker container")
		}
	}

	return RuntimeContext{Target: addr, Stop: stop}, nil
}
