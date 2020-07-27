// +build !windows

package disk

import (
	"syscall"

	"massnet.org/mass/logging"
)

func init() {
	CheckDiskSpaceStub = CheckDiskSpaceNonWin
}

func CheckDiskSpaceNonWin(path string, additional uint64) bool {
	stat := syscall.Statfs_t{}
	err := syscall.Statfs(path, &stat)
	if err != nil {
		logging.CPrint(logging.ERROR, "CheckDiskSpace error", logging.LogFormat{"err": err, "path": path})
		return false
	}
	free := stat.Bfree * uint64(stat.Bsize)
	logging.CPrint(logging.DEBUG, "CheckDiskSpace", logging.LogFormat{"available": free, "path": path})
	return free >= additional+MinDiskSpace
}
