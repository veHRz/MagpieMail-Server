package config_test

import (
	"fmt"

	"github.com/veHRz/MagpieMail-Server/internal/config"
)

func ExampleLoad_invalidConfiguration() {
	_, err := config.Load(config.Source{Environ: []string{
		"MAGPIE_LOG__LEVEL=verbose",
		"MAGPIE_SERVER__LISEN=:8080",
		"MAGPIE_SERVER__READ_TIMEOUT=30",
	}})
	fmt.Println(err)
	// Output:
	// invalid configuration:
	//   - MAGPIE_SERVER__LISEN: unknown configuration variable (no setting named "server.lisen")
	//   - database.url: is required
	//   - log.level (from MAGPIE_LOG__LEVEL): must be one of debug, info, warn, error, got "verbose"
	//   - server.read_timeout (from MAGPIE_SERVER__READ_TIMEOUT): must be a duration with a unit, such as "30s" or "2m"
}
