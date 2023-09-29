// Package main implements xrender, a CLI tool for rendering XRs.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/alecthomas/kong"
	"github.com/fsnotify/fsnotify"
	"sigs.k8s.io/yaml"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/composed"

	v1 "github.com/crossplane/crossplane/apis/apiextensions/v1"
)

// CLI arguments and flags for xrender.
type CLI struct {
	Debug             bool          `short:"d" help:"Emit debug logs in addition to info logs."`
	Timeout           time.Duration `help:"How long to run before timing out." default:"3m"`
	Watch             bool          `short:"w" help:"Live reload files."`
	CompositeResource string        `arg:"" type:"existingfile" help:"A YAML manifest containing the Composite Resource (XR) to render."`
	Composition       string        `arg:"" type:"existingfile" help:"A YAML manifest containing the Composition to use. Must be mode: Pipeline."`
	Functions         string        `arg:"" type:"existingfile" help:"A YAML stream of manifests containing the Composition Functions to use."`

	ObservedResources string `short:"o" type:"existingfile" help:"An optional YAML stream of manifests mocking the observed state of composed resources."`
}

type Rendered struct {
	CLI

	Watcher fsnotify.Watcher
}

type RenderFunc func(*Rendered) error

// Run xrender.
func (c *CLI) Run() error {
	r := &Rendered{
		CLI: *c,
	}
	err := r.render()
	if err != nil {
		return err
	}
	if c.Watch {
		watcher, err := NewWatcher(r)
		if err != nil {
			return errors.Wrapf(err, "cannot create file watch %q", err)
		}
		defer watcher.Close()
		done := make(chan bool)
		go func() {
			for {
				select {
				case event := <-watcher.Events:
					if event.Has(fsnotify.Write) {
						r.render()
					}
				case err := <-watcher.Errors:
					fmt.Fprintf(os.Stderr, "Error %s", err)
					os.Exit(-1)
				}
			}
		}()
		<-done
		os.Exit(0)
	}
	return nil
}
func (r *Rendered) render() error { //nolint:gocyclo // Only a touch over.
	xr, err := LoadCompositeResource(r.CompositeResource)
	if err != nil {
		return errors.Wrapf(err, "cannot load composite resource from %q", r.CompositeResource)
	}

	// TODO(negz): Should we do some simple validations, e.g. that the
	// Composition's compositeTypeRef matches the XR's type?
	comp, err := LoadComposition(r.Composition)
	if err != nil {
		return errors.Wrapf(err, "cannot load Composition from %q", r.Composition)
	}

	if m := comp.Spec.Mode; m == nil || *m != v1.CompositionModePipeline {
		return errors.Errorf("xrender only supports Composition Function pipelines: Composition %q must use spec.mode: Pipeline", comp.GetName())
	}

	fns, err := LoadFunctions(r.Functions)
	if err != nil {
		return errors.Wrapf(err, "cannot load functions from %q", r.Functions)
	}

	ors := []composed.Unstructured{}
	if r.ObservedResources != "" {
		ors, err = LoadObservedResources(r.ObservedResources)
		if err != nil {
			return errors.Wrapf(err, "cannot load observed composed resources from %q", r.ObservedResources)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.Timeout)
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

	y, err := yaml.Marshal(out.CompositeResource.GetUnstructured())
	if err != nil {
		return errors.Wrapf(err, "cannot marshal composite resource %q to YAML", xr.GetName())
	}
	fmt.Printf("---\n%s", y)

	for _, cd := range out.ComposedResources {
		y, err := yaml.Marshal(cd.GetUnstructured())
		if err != nil {
			// TODO(negz): Use composed name annotation instead.
			return errors.Wrapf(err, "cannot marshal composed resource %q to YAML", cd.GetName())
		}
		fmt.Printf("---\n%s", y)
	}

	return nil
}

func main() {
	ctx := kong.Parse(&CLI{}, kong.Description("Render an XR using Composition Functions."))
	ctx.FatalIfErrorf(ctx.Run())
}
