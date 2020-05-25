package disk

import (
	"fmt"
	"os"
	"path/filepath"
)

type FlatFileSeq struct {
	dir       string
	prefix    string
	chunkSize int64
}

func NewFlatFileSeq(dir, prefix string, chunkSize int64) *FlatFileSeq {
	return &FlatFileSeq{
		dir:       dir,
		prefix:    prefix,
		chunkSize: chunkSize,
	}
}

func (f *FlatFileSeq) FilePath(pos *FlatFilePos) string {
	return filepath.Join(f.dir, fmt.Sprintf("%s%05d.dat", f.prefix, pos.fileNo))
}

func (f *FlatFileSeq) ExistFile(pos *FlatFilePos) (bool, error) {
	_, err := os.Stat(f.FilePath(pos))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (f *FlatFileSeq) Open(pos *FlatFilePos, readOnly bool) (file *os.File, err error) {
	if pos == nil {
		return nil, ErrInvalidFlatFilePos
	}

	path := f.FilePath(pos)
	if readOnly {
		file, err = os.Open(path)
	} else {
		file, err = os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0755)
	}
	if err != nil {
		return
	}
	_, err = file.Seek(pos.pos, os.SEEK_SET)
	if err != nil {
		file.Close()
		file = nil
	}
	return
}

func (f *FlatFileSeq) Allocate(pos *FlatFilePos, size int64) error {
	oldChunks := (pos.pos + f.chunkSize - 1) / f.chunkSize
	newChunks := (pos.pos + f.chunkSize - 1 + size) / f.chunkSize
	if newChunks <= oldChunks {
		return nil
	}
	inc := newChunks*f.chunkSize - pos.pos
	file, err := f.Open(pos, false)
	if err != nil {
		return err
	}
	defer file.Close()
	if CheckDiskSpaceStub(f.dir, uint64(inc)) {
		return TruncateFile(file, pos.pos+inc)
	}
	return ErrOutOfSpace
}
