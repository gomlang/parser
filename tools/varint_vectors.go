package main

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"os"
)

func main() {
	f, _ := os.Create("tests/varint_reference_test.goml")
	defer f.Close()
	fmt.Fprintln(f, "package tests;\nuse ecosystem::parser::binary;\nuse std::testing;\n#[test]\nfn go_varint_vectors() -> () {")
	r := rand.New(rand.NewSource(42))
	v := []uint64{0, 1, 127, 128, 255, 256, 1 << 63, ^uint64(0)}
	for i := 0; i < 128; i++ {
		v = append(v, r.Uint64())
	}
	for _, x := range v {
		b := binary.AppendUvarint(nil, x)
		fmt.Fprint(f, "testing::assert_eq(testing::expect_ok(binary::uleb128().parse(b\"")
		for _, c := range b {
			fmt.Fprintf(f, "\\x%02x", c)
		}
		fmt.Fprintf(f, "\".as_slice()), \"Go Uvarint\"), %d);\n", x)
		s, _ := binary.Varint(b)
		fmt.Fprint(f, "testing::assert_eq(testing::expect_ok(binary::zigzag64().parse(b\"")
		for _, c := range b {
			fmt.Fprintf(f, "\\x%02x", c)
		}
		if s == -1<<63 {
			fmt.Fprintln(f, "\".as_slice()), \"Go Varint\"), -9223372036854775807 - 1);")
		} else {
			fmt.Fprintf(f, "\".as_slice()), \"Go Varint\"), %d);\n", s)
		}
	}
	fmt.Fprintln(f, "}")
}
