// +build !windows

package massdb_v1

import (
	"os"
)

// Expand file to actual size
func expandMapFile(f *os.File, typ MapType, bl int) error {
	//fileSize := calcSize(typ, bl)
	//_, err := f.WriteAt([]byte{byte(0)}, int64(fileSize)-1)
	//return err
	return nil
}
