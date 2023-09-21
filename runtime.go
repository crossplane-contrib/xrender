package main

import (
	"context"

	pkgv1beta1 "github.com/crossplane/crossplane/apis/pkg/v1beta1"
)

// A Runtime runs a Function.
type Runtime interface {
	// Start the Function.
	Start(ctx context.Context) (RuntimeContext, error)
}

// RuntimeContext contains context on how a Function is being run.
type RuntimeContext struct {
	// Target for RunFunctionRequest gRPCs.
	Target string

	// Stop the running Function
	Stop func(context.Context) error
}

// GetRuntime for the supplied Function, per its annotations.
func GetRuntime(fn pkgv1beta1.Function) (Runtime, error) { //nolint:unparam // We will likely want an error here in future for other GetRuntime variants.
	// TODO(negz): Support other runtimes based on annotation.
	return GetRuntimeDocker(fn), nil
}
