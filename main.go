// Package main implements xrender, a CLI tool for rendering XRs.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/alecthomas/kong"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/composed"

	v1 "github.com/crossplane/crossplane/apis/apiextensions/v1"
)

// CLI arguments and flags for xrender.
type CLI struct {
	Debug   bool          `short:"d" help:"Emit debug logs in addition to info logs."`
	Timeout time.Duration `help:"How long to run before timing out." default:"3m"`

	CompositeResource string `arg:"" type:"existingfile" help:"A YAML manifest containing the Composite Resource (XR) to render."`
	Composition       string `arg:"" type:"existingfile" help:"A YAML manifest containing the Composition to use. Must be mode: Pipeline."`
	Functions         string `arg:"" help:"A stream or directory of YAML manifests containing the Composition Functions to use."`

	ObservedResources []string `short:"o" help:"An optional stream or directory of YAML manifests mocking the observed state of composed resources."`
	IncludeResults    bool     `short:"r" help:"Include Results in the output. Results are emitted as a 'fake' KRM-like object of kind: Result."`
}

// Run xrender.
func (c *CLI) Run() error { //nolint:gocyclo // Only a touch over.
	xr, err := LoadCompositeResource(c.CompositeResource)
	if err != nil {
		return errors.Wrapf(err, "cannot load composite resource from %q", c.CompositeResource)
	}

	// TODO(negz): Should we do some simple validations, e.g. that the
	// Composition's compositeTypeRef matches the XR's type?
	comp, err := LoadComposition(c.Composition)
	if err != nil {
		return errors.Wrapf(err, "cannot load Composition from %q", c.Composition)
	}

	if m := comp.Spec.Mode; m == nil || *m != v1.CompositionModePipeline {
		return errors.Errorf("xrender only supports Composition Function pipelines: Composition %q must use spec.mode: Pipeline", comp.GetName())
	}

	fns, err := LoadFunctions(c.Functions)
	if err != nil {
		return errors.Wrapf(err, "cannot load functions from %q", c.Functions)
	}

	ors := []composed.Unstructured{}
	for i := range c.ObservedResources {
		ors, err = LoadObservedResources(c.ObservedResources[i])
		if err != nil {
			return errors.Wrapf(err, "cannot load observed composed resources from %q", c.ObservedResources[i])
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()

	out, err := Render(ctx, RenderInputs{
		CompositeResource: xr,
		Composition:       comp,
		Functions:         fns,
		ObservedResources: ors,
	})
	if err != nil {
		return errors.Wrap(err, "cannot render composite resource")
	}

	// TODO(negz): Right now we're just emitting the desired state, which is an
	// overlay on the observed state. Would it be more useful to apply the
	// overlay to show something more like what the final result would be? The
	// challenge with that would be that we'd have to try emulate what
	// server-side apply would do (e.g. merging vs atomically replacing arrays)
	// and we don't have enough context (i.e. OpenAPI schemas) to do that.

	s := json.NewSerializerWithOptions(json.DefaultMetaFactory, nil, nil, json.SerializerOptions{Yaml: true})

	fmt.Println("---")
	if err := s.Encode(out.CompositeResource, os.Stdout); err != nil {
		return errors.Wrapf(err, "cannot marshal composite resource %q to YAML", xr.GetName())
	}

	for i := range out.ComposedResources {
		fmt.Println("---")
		if err := s.Encode(&out.ComposedResources[i], os.Stdout); err != nil {
			// TODO(negz): Use composed name annotation instead.
			return errors.Wrapf(err, "cannot marshal composed resource %q to YAML", out.ComposedResources[i].GetName())
		}
	}

	if c.IncludeResults {
		for i := range out.Results {
			fmt.Println("---")
			if err := s.Encode(&out.Results[i], os.Stdout); err != nil {
				return errors.Wrap(err, "cannot marshal result to YAML")
			}
		}
	}

	return nil
}

func main() {
	ctx := kong.Parse(&CLI{}, kong.Description("Render an XR using Composition Functions."))
	ctx.FatalIfErrorf(ctx.Run())
}
