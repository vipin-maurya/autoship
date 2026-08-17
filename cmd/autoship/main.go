// Command autoship polls an Android repo and ships closed-testing releases unattended.
package main

import (
	"os"

	"github.com/vipinm/autoship/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
