package host

import (
	"io/fs"

	adminpublic "github.com/assurrussa/goadmin/public"
	adminviews "github.com/assurrussa/goadmin/views"
)

// CoreTemplateFS returns the embedded reusable goadmin templates.
func CoreTemplateFS() fs.FS {
	return adminviews.Templates
}

// CorePublicFS returns the embedded reusable goadmin public assets.
func CorePublicFS() fs.ReadFileFS {
	return adminpublic.Files
}
