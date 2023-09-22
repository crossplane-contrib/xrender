package main

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"

	"github.com/crossplane/crossplane-runtime/pkg/errors"

	pkgv1beta1 "github.com/crossplane/crossplane/apis/pkg/v1beta1"
)

// Annotations that can be used to configure the Docker runtime.
const (
	// AnnotationKeyRuntimeDockerCleanup configures how a Function's Docker
	// container should be cleaned up once rendering is done.
	AnnotationKeyRuntimeDockerCleanup = "xrender.crossplane.io/runtime-docker-cleanup"

	// AnnotationKeyRuntimeDockerImage overrides the Docker image that will be
	// used to run the Function. By default xrender assumes the Function package
	// (i.e. spec.package) can be used to run the Function.
	AnnotationKeyRuntimeDockerImage = "xrender.crossplane.io/runtime-docker-image"
)

// Supported AnnotationKeyRuntimeDockerCleanup values.
const (
	// AnnotationValueRuntimeDockerCleanupStop is the default. It stops the
	// container once rendering is done.
	AnnotationValueRuntimeDockerCleanupStop = "Stop"

	// AnnotationValueRuntimeDockerCleanupOrphan leaves the container running
	// once rendering is done.
	AnnotationValueRuntimeDockerCleanupOrphan = "Orphan"
)

// RuntimeDocker uses a Docker daemon to run a Function.
type RuntimeDocker struct {
	// Image to run
	Image string

	// Stop container once rendering is done
	Stop bool
}

// GetRuntimeDocker extracts RuntimeDocker configuration from the supplied
// Function.
func GetRuntimeDocker(fn pkgv1beta1.Function) *RuntimeDocker {
	// TODO(negz): Pull package in case it has a different controller image? I
	// hope in most cases Functions will use 'fat' packages, and it's possible
	// to manually override with an annotation so maybe not worth it.
	r := &RuntimeDocker{
		Image: fn.Spec.Package,
		Stop:  true,
	}
	if i := fn.GetAnnotations()[AnnotationKeyRuntimeDockerImage]; i != "" {
		r.Image = i
	}
	if fn.GetAnnotations()[AnnotationKeyRuntimeDockerCleanup] == AnnotationValueRuntimeDockerCleanupOrphan {
		r.Stop = false
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

	if found := strings.Contains(r.Image, "/"); found {
		out, err := c.ImagePull(ctx, r.Image, types.ImagePullOptions{})
		if err != nil {
			return RuntimeContext{}, errors.Wrapf(err, "cannot pull Docker image %q", r.Image)
		}
		defer out.Close() //nolint:errcheck // TODO(negz): Can this error?
	}

	// Find a random, available port. There's a chance of a race here, where
	// something else binds to the port before we start our container.
	lis, err := net.Listen("tcp", "localhost:0")
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

	// TODO(negz): Set a container name? Presumably unique across runs.
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
