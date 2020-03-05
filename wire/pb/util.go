package wirepb

import (
	"bytes"
	"io"
	"math/rand"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func writeBytes(w io.Writer, elements ...[]byte) (int, error) {
	var count = 0
	for _, e := range elements {
		n, err := w.Write(e)
		count += n
		if err != nil {
			return count, err
		}
	}
	return count, nil
}

func moveBytes(bs []byte) []byte {
	if len(bs) == 0 {
		return make([]byte, 0)
	}
	return bs
}

func combineBytes(elements ...[]byte) []byte {
	return bytes.Join(elements, nil)
}
