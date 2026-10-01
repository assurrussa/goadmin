// Package host contains the reusable host-adapter v1 surface for embedding
// the goadmin subsystem into a Go backend.
//
// The package intentionally keeps the public API narrow:
// callers map their own typed config into the exported input structs, provide
// their own feature registry implementation, and keep project-specific runtime
// dependencies outside this package.
//
// Built-in admin auth, roles, seeders, first-admin setup tokens, and default
// repositories stay inside goadmin; host projects integrate them through the
// stable facade rather than wiring internal packages directly.
package host
