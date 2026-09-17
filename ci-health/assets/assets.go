package assets

import _ "embed"

// FuseJS is the pinned Fuse.js browser module used by the registry search.
//
//go:embed vendor/fuse/fuse.min.mjs
var FuseJS []byte
