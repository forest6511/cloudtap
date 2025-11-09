# internal/

This tree holds packages that must not be imported outside the `cloudtap`
module. Use it for helpers such as control-plane caches, feature-flag wiring,
and secrets management. Everything here should be documented via package-level
comments so contributors know whether to extend or duplicate logic elsewhere.
