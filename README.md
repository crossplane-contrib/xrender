# xrender

Locally render any XR that uses Composition Functions.

```shell
# Install xrender
$ go install github.com/negz/xrender@latest

# See how to use it
$ xrender --help
Usage: xrender <composite-resource> <composition> <functions>

Render an XR using Composition Functions.

Arguments:
  <composite-resource>    A YAML manifest containing the Composite Resource (XR) to render.
  <composition>           A YAML manifest containing the Composition to use. Must be mode: Pipeline.
  <functions>             A YAML stream of manifests containing the Composition Functions to use.

Flags:
  -h, --help                         Show context-sensitive help.
  -d, --debug                        Emit debug logs in addition to info logs.
      --timeout=3m                   How long to run before timing out.
  -o, --observed-resources=STRING    An optional YAML stream of manifests mocking the observed state of composed resources.

# Try it out using the examples in this repository
$ xrender examples/xr.yaml examples/composition.yaml examples/functions.yaml
---
apiVersion: nopexample.org/v1
kind: XBucket
metadata:
  name: test-xrender
status:
  bucketRegion: us-east-2
---
apiVersion: s3.aws.upbound.io/v1beta1
kind: Bucket
metadata:
  annotations:
    crossplane.io/composition-resource-name: my-bucket
  generateName: test-xrender-
  labels:
    crossplane.io/composite: test-xrender
  ownerReferences:
  - apiVersion: nopexample.org/v1
    blockOwnerDeletion: true
    controller: true
    kind: XBucket
    name: test-xrender
    uid: ""
spec:
  forProvider:
    region: us-east-2
```

## How does it work?

`xrender` only supports Compositions in `mode: Pipeline` - i.e. Compositions
that are powered by Composition Functions. It works by:

1. Running the pipeline of Composition Functions locally.
1. Running your XR through the pipeline.
1. Printing the results to stdout.

You can also pass the `-o` flag to pass a series of "observed composed
resources" to the pipeline along with your XR. This is useful to see how your
Composition would work if some of its composed resources already existed, for
example to test out copying composed resource status back to the XR.

By default `xrender` uses Docker to run Functions locally.

## Configuration

`xrender` uses "runtimes" to run Functions. It's designed to easily be extended
with new runtimes. Right now it supports two:

* Docker (default) - run Functions using a Docker daemon.
* Development - connect to a Function running locally (e.g. using `go run`).

You can configure which runtime to use on a Function-by-Function basis. This is
handy for developing Functions. Say your Composition uses a pipeline of three
Functions, and you're developing one of them. You can run the two "stable"
Functions using the Docker runtime, and use the Development runtime to tell
`xrender` to connect to the Function you're running locally.

You configure what runtime to use by adding annotations to the Function packages
in `functions.yaml`. The following annotations are supported:

* `xrender.crossplane.io/runtime: Docker` (default) - Use the Docker runtime.
* `xrender.crossplane.io/runtime: Development` - Use the Development runtime.

The Docker runtime supports the following additional annotations:

* `xrender.crossplane.io/runtime-docker-cleanup: Stop` (default) - Stop the
  Docker container after rendering the XR.
* `xrender.crossplane.io/runtime-docker-cleanup: Orphan` - Leave the Docker
  container running after rendering the XR.
* `xrender.crossplane.io/runtime-docker-image` - Override the image used to run
  the Function. The Function's `spec.package` is used by default.

For example:

```yaml
---
apiVersion: pkg.crossplane.io/v1beta1
kind: Function
metadata:
  name: function-dummy
  annotations:
    xrender.crossplane.io/runtime: Docker
    xrender.crossplane.io/runtime-docker-cleanup: Orphan
    xrender.crossplane.io/runtime-docker-image: 690d376c2b2a # Some local image build.
spec:
  package: xpkg.upbound.io/crossplane-contrib/function-dummy:v0.2.1
```

The Development runtime supports the following additional annotation:

* `xrender.crossplane.io/runtime-development-target` - The gRPC 'target' address
  at which the Function is listening. The default is `localhost:9443`.

Note that the Development requires the Function to be listening in `--insecure`
mode, i.e. without mTLS transport security.

For example:

```yaml
---
apiVersion: pkg.crossplane.io/v1beta1
kind: Function
metadata:
  name: function-dummy
  annotations:
    xrender.crossplane.io/runtime: Development
    xrender.crossplane.io/runtime-development-target: localhost:9443 # A Function running locally
spec:
  package: xpkg.upbound.io/crossplane-contrib/function-dummy:v0.2.1
```