package snowflake

import (
	"bytes"
	"testing"
)

func TestMarshalJSON(t *testing.T) {
	id := ID(13587)
	expected := "\"13587\""

	bytes, err := id.MarshalJSON()
	if err != nil {
		t.Error("Unexpected error during MarshalJSON")
	}

	if string(bytes) != expected {
		t.Errorf("Got %s, expected %s", string(bytes), expected)
	}
}

func TestMarshalsIntBytes(t *testing.T) {
	id := ID(13587).IntBytes()
	expected := []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x35, 0x13}
	if !bytes.Equal(id[:], expected) {
		t.Errorf("Expected ID to be encoded as %v, got %v", expected, id)
	}
}

func TestUnmarshalJSON(t *testing.T) {
	strID := "\"13587\""
	expected := ID(13587)

	var id ID
	err := id.UnmarshalJSON([]byte(strID))
	if err != nil {
		t.Error("Unexpected error during UnmarshalJSON")
	}

	if id != expected {
		t.Errorf("Got %d, expected %d", id, expected)
	}
}

func TestBase58(t *testing.T) {

	node, _ := NewNode(1)

	for i := 0; i < 10; i++ {

		sf := node.Generate()
		b58 := sf.Base58()
		psf, err := ParseBase58([]byte(b58))
		if err != nil {
			t.Fatal(err)
		}
		if sf != psf {
			t.Fatal("Parsed does not match String.")
		}
	}
}
func BenchmarkParseBase58(b *testing.B) {

	node, _ := NewNode(1)
	sf := node.Generate()
	b58 := sf.Base58()

	b.ReportAllocs()

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		ParseBase58([]byte(b58))
	}
}
func BenchmarkBase58(b *testing.B) {

	node, _ := NewNode(1)
	sf := node.Generate()

	b.ReportAllocs()

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		sf.Base58()
	}
}
func BenchmarkGenerate(b *testing.B) {

	node, _ := NewNode(1)

	b.ReportAllocs()

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		_ = node.Generate()
	}
}

func BenchmarkUnmarshal(b *testing.B) {
	// Generate the ID to unmarshal
	node, _ := NewNode(1)
	id := node.Generate()
	bytes, _ := id.MarshalJSON()

	var id2 ID

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		_ = id2.UnmarshalJSON(bytes)
	}
}

func BenchmarkMarshal(b *testing.B) {
	// Generate the ID to marshal
	node, _ := NewNode(1)
	id := node.Generate()

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		_, _ = id.MarshalJSON()
	}
}
