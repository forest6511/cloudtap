# plugins/

Prototype hooks that respond to tunnel lifecycle events live here. Each plugin
should ship its own subdirectory containing Go code (when embeddable) or a
container definition when the hook executes out-of-process. Keep experimental
artifacts in this tree until the SDK is finalized.
