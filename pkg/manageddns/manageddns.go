package manageddns

import (
	"bufio"
	"os"
	"strings"

	"github.com/openshift/hive/pkg/constants"
)

// ReadManagedDomainsFile reads the managed domains from the file pointed to
// by the ManagedDomainsFileEnvVar environment variable.
func ReadManagedDomainsFile() ([]string, error) {
	managedDomainsFile := os.Getenv(constants.ManagedDomainsFileEnvVar)
	if len(managedDomainsFile) == 0 {
		return nil, nil
	}
	file, err := os.Open(managedDomainsFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	domains := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		s := scanner.Text()
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		domains = append(domains, s)
	}
	return domains, scanner.Err()
}
