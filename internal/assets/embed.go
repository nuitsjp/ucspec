package assets

import "embed"

//go:embed template schemas viewer authoring.md manual.md
var Files embed.FS
