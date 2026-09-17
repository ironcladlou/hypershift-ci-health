package assets

import "embed"

// Files contains the browser application and its pinned third-party modules.
//
//go:embed web vendor
var Files embed.FS
