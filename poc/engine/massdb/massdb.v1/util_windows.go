// +build windows

package massdb_v1

import (
	"os"
)

// do not expand map file
func expandMapFile(f *os.File, typ MapType, bl int) error {
	return nil
}
