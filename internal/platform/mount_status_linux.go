package platform

import (
	"os"
	"path/filepath"
	"strings"
)

func FolderMounted(path string) bool {
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return true
	}
	decode := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 6 && filepath.Clean(decode.Replace(fields[4])) == filepath.Clean(path) {
			return true
		}
	}
	return false
}
