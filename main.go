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

// Run xrender.
func (c *CLI) Run() error {
	ri, err := Initialize(c)
	if err != nil {
		return errors.Wrapf(err, "cannot initialize %q", err)
	}
	err = RenderInput(ri)
	if err != nil {
		return err
	}
	if c.Watch {
		watcher, err := NewWatcher(c)
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
						ri.Update(c, event.Name)
						err := RenderInput(ri)
						if err != nil {
							fmt.Fprintf(os.Stderr, "Error %s", err)
						}
					}
				case err := <-watcher.Errors:
					fmt.Fprintf(os.Stderr, "Error %s", err)
					os.Exit(-1)
				}
			}
		}()
		<-done
	}
	return nil
}
func Initialize(c *CLI) (RenderInputs, error) {
	ri := RenderInputs{}
	for _, f := range []string{c.CompositeResource, c.Composition, c.Functions, c.ObservedResources} {
		err := ri.Update(c, f)
		if err != nil {
			return ri, err
		}
	}

	return ri, nil
}
func (ri *RenderInputs) Update(c *CLI, filename string) error {
	switch filename {
	case c.CompositeResource:
		xr, err := LoadCompositeResource(c.CompositeResource)
		if err != nil {
			return errors.Wrapf(err, "cannot load composite resource from %q", c.CompositeResource)
		}
		ri.CompositeResource = xr
	case c.Composition:
		comp, err := LoadComposition(c.Composition)
		if err != nil {
			return errors.Wrapf(err, "cannot load Composition from %q", c.Composition)
		}
		if m := comp.Spec.Mode; m == nil || *m != v1.CompositionModePipeline {
			return errors.Errorf("xrender only supports Composition Function pipelines: Composition %q must use spec.mode: Pipeline", comp.GetName())
		}
		ri.Composition = comp
	case c.Functions:
		fns, err := LoadFunctions(c.Functions)
		if err != nil {
			return errors.Wrapf(err, "cannot load functions from %q", c.Functions)
		}
		ri.Functions = fns
	case c.ObservedResources:
		ors := []composed.Unstructured{}
		if c.ObservedResources != "" {
			var err error
			ors, err = LoadObservedResources(c.ObservedResources)
			if err != nil {
				return errors.Wrapf(err, "cannot load observed composed resources from %q", c.ObservedResources)
			}
		}
		ri.ObservedResources = ors
	}
	return nil
}

func RenderInput(ri RenderInputs) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	out, err := Render(ctx, ri)
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
		return errors.Wrapf(err, "cannot marshal composite resource %q to YAML", ri.CompositeResource.GetName())
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
