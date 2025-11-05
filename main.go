package main

import (
	"embed"
	"os"

	"github.com/rmitchellscott/rm-qmd-verify/cmd"
)

//go:embed ui/dist
//go:embed ui/dist/assets
var EmbeddedUI embed.FS

func main() {
	cmd.SetEmbeddedUI(EmbeddedUI)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
