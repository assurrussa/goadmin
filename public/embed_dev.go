//go:build dev

package public

import "embed"

// Files is empty in dev to avoid embedding missing build artifacts.
var Files embed.FS
