// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sha256_test

import (
	"fmt"
	"io"
	"log"
	"os"

	"massnet.org/mass/poc/pocutil/crypto/sha256"
)

func ExampleSum256() {
	sum := sha256.Sum256([]byte("hello world\n"))
	fmt.Printf("%x", sum)
	// Output: e44e5c872bedc8aead95e5a63d6c31cb10be0caeb47e80cc7aecab64ffaab24f
}

func ExampleNew() {
	h := sha256.New()
	h.Write([]byte("hello world\n"))
	fmt.Printf("%x", h.Sum(nil))
	// Output: e44e5c872bedc8aead95e5a63d6c31cb10be0caeb47e80cc7aecab64ffaab24f
}

func ExampleNew_file() {
	f, err := os.Open("file.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%x", h.Sum(nil))
}
