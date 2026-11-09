// Command loadl is the Loadlore CLI.
package main

import (
	"os"

	"github.com/akinfemi/loadlore/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
