// Command loadl is the Loadlore CLI.
package main

import (
	"os"

	"github.com/akinfemi/load-lore/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
