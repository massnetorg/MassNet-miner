package massutil

import (
	"encoding/hex"
	"fmt"
	"testing"
)

func ExampleSha256() {
	data := []byte("test hash256")
	fmt.Println(hex.EncodeToString(Sha256(data)))

	// Output:
	// de8503647d0760bbabc8bf47526176bd1046afa9f5f20d8831d0ff455cee0523
}

func ExampleHash256() {
	data := []byte("test hash256")
	fmt.Println(hex.EncodeToString(Hash256(data)))

	// Output:
	// cb43cc5fc9e305ddf8fccc2112629da4d21fc840937b785e86d4a220406359a8
}

func ExampleRipemd160() {
	data := []byte("test hash256")
	fmt.Println(hex.EncodeToString(Ripemd160(data)))

	// Output:
	// 07fc1824f3c8b5c0aebfe9edd7b519a85def76eb
}

func ExampleHash160() {
	data := []byte("test hash160")
	fmt.Println(hex.EncodeToString(Hash160(data)))

	// Output:
	// b720061a734285a70e86cb32b31f32884e198c32
}

// BenchmarkSha256-8   	 5000000	       245 ns/op	      32 B/op	       1 allocs/op
func BenchmarkSha256(b *testing.B) {
	data := []byte("bench sha256")

	for i := 0; i < b.N; i++ {
		Sha256(data)
	}
}

// BenchmarkHash256-8   	 3000000	       469 ns/op	      32 B/op	       1 allocs/op
func BenchmarkHash256(b *testing.B) {
	data := []byte("bench hash256")

	for i := 0; i < b.N; i++ {
		Hash256(data)
	}
}

// BenchmarkRipemd160-8   	 2000000	       762 ns/op	     144 B/op	       2 allocs/op
func BenchmarkRipemd160(b *testing.B) {
	data := []byte("bench ripemd160")

	for i := 0; i < b.N; i++ {
		Ripemd160(data)
	}
}

// BenchmarkHash160-8   	 1000000	      1032 ns/op	     176 B/op	       3 allocs/op
func BenchmarkHash160(b *testing.B) {
	data := []byte("bench hash160")

	for i := 0; i < b.N; i++ {
		Hash160(data)
	}
}
