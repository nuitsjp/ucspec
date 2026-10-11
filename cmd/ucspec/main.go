package main

import (
	"context"
	"fmt"
	"github.com/nuitsjp/ucspec/internal/adapters"
	"github.com/nuitsjp/ucspec/internal/app"
	"github.com/nuitsjp/ucspec/internal/assets"
	"github.com/nuitsjp/ucspec/internal/buildinfo"
	"os"
)

func main() {
	codec, err := adapters.Codec(assets.Files)
	if err != nil {
		fmt.Fprintln(os.Stderr, "embedded schema initialization failed:", err)
		os.Exit(2)
	}
	os.Exit(app.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, app.Dependencies{Codec: codec, Layout: adapters.LayoutJSON, Assets: assets.Files, Version: buildinfo.Version, Repository: buildinfo.Repository}))
}
