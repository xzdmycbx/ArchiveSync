// Command archive-sync is the ArchiveSync backup-sync server and admin CLI.
package main

import (
	// Embed the IANA timezone database so time.LoadLocation resolves named
	// zones (e.g. "Asia/Shanghai") even on minimal deploys (scratch/distroless
	// containers, hosts without system zoneinfo). Scheduling and retention day
	// boundaries depend on this.
	_ "time/tzdata"

	"archivesync/internal/cli"
)

func main() {
	cli.Execute()
}
