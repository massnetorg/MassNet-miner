// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hash_test

import (
	"bytes"
	"encoding"
	"fmt"
	"log"

	"massnet.org/mass/poc/pocutil/crypto/sha256"
)

func Example_binaryMarshaler() {
	const (
		input1 = "The tunneling gopher digs downwards, "
		input2 = "unaware of what he will find."
	)

	first := sha256.New()
	first.Write([]byte(input1))

	marshaler, ok := first.(encoding.BinaryMarshaler)
	if !ok {
		log.Fatal("first does not implement encoding.BinaryMarshaler")
	}
	state, err := marshaler.MarshalBinary()
	if err != nil {
		log.Fatal("unable to marshal hash:", err)
	}

	second := sha256.New()

	unmarshaler, ok := second.(encoding.BinaryUnmarshaler)
	if !ok {
		log.Fatal("second does not implement encoding.BinaryUnmarshaler")
	}
	if err := unmarshaler.UnmarshalBinary(state); err != nil {
		log.Fatal("unable to unmarshal hash:", err)
	}

	first.Write([]byte(input2))
	second.Write([]byte(input2))

	fmt.Printf("%x\n", first.Sum(nil))
	fmt.Println(bytes.Equal(first.Sum(nil), second.Sum(nil)))
	// Output:
	// 8687d9eb9df1b20a4617e8cfe73f8886bee1dc78a08ae28519c4b89db4abbea7
	// true
}
