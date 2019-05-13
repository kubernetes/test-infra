// Package snowflake provides a very simple Twitter snowflake generator and parser.
package snowflake

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"
)

var (
	// Epoch is set to the twitter snowflake epoch of Nov 04 2010 01:42:54 UTC
	// You may customize this to set a different epoch for your application.
	Epoch int64 = 1288834974657

	// Number of bits to use for Node
	// Remember, you have a total 22 bits to share between Node/Step
	NodeBits uint8 = 10

	// Number of bits to use for Step
	// Remember, you have a total 22 bits to share between Node/Step
	StepBits uint8 = 12

	nodeMax   int64 = -1 ^ (-1 << NodeBits)
	nodeMask  int64 = nodeMax << StepBits
	stepMask  int64 = -1 ^ (-1 << StepBits)
	timeShift uint8 = NodeBits + StepBits
	nodeShift uint8 = StepBits
)

const encodeBase32Map = "ybndrfg8ejkmcpqxot1uwisza345h769"

var decodeBase32Map [256]byte

const encodeBase58Map = "123456789abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ"

var decodeBase58Map [256]byte

// A JSONSyntaxError is returned from UnmarshalJSON if an invalid ID is provided.
type JSONSyntaxError struct{ original []byte }

func (j JSONSyntaxError) Error() string {
	return fmt.Sprintf("invalid snowflake ID %q", string(j.original))
}

// Create a map for decoding Base58.  This speeds up the process tremendously.
func init() {

	for i := 0; i < len(encodeBase58Map); i++ {
		decodeBase58Map[i] = 0xFF
	}

	for i := 0; i < len(encodeBase58Map); i++ {
		decodeBase58Map[encodeBase58Map[i]] = byte(i)
	}

	for i := 0; i < len(encodeBase32Map); i++ {
		decodeBase32Map[i] = 0xFF
	}

	for i := 0; i < len(encodeBase32Map); i++ {
		decodeBase32Map[encodeBase32Map[i]] = byte(i)
	}
}

// ErrInvalidBase58 is returned by ParseBase58 when given an invalid []byte
var ErrInvalidBase58 = errors.New("invalid base58")

// ErrInvalidBase32 is returned by ParseBase32 when given an invalid []byte
var ErrInvalidBase32 = errors.New("invalid base32")

// A Node struct holds the basic information needed for a snowflake generator
// node
type Node struct {
	mu   sync.Mutex
	time int64
	node int64
	step int64
}

// An ID is a custom type used for a snowflake ID.  This is used so we can
// attach methods onto the ID.
type ID int64

// NewNode returns a new snowflake node that can be used to generate snowflake
// IDs
func NewNode(node int64) (*Node, error) {

	// re-calc in case custom NodeBits or StepBits were set
	nodeMax = -1 ^ (-1 << NodeBits)
	nodeMask = nodeMax << StepBits
	stepMask = -1 ^ (-1 << StepBits)
	timeShift = NodeBits + StepBits
	nodeShift = StepBits
	
	if node < 0 || node > nodeMax {
		return nil, errors.New("Node number must be between 0 and " + strconv.FormatInt(nodeMax, 10))
	}

	return &Node{
		time: 0,
		node: node,
		step: 0,
	}, nil
}

// Generate creates and returns a unique snowflake ID
func (n *Node) Generate() ID {

	n.mu.Lock()

	now := time.Now().UnixNano() / 1000000

	if n.time == now {
		n.step = (n.step + 1) & stepMask

		if n.step == 0 {
			for now <= n.time {
				now = time.Now().UnixNano() / 1000000
			}
		}
	} else {
		n.step = 0
	}

	n.time = now

	r := ID((now-Epoch)<<timeShift |
		(n.node << nodeShift) |
		(n.step),
	)

	n.mu.Unlock()
	return r
}

// Int64 returns an int64 of the snowflake ID
func (f ID) Int64() int64 {
	return int64(f)
}

// String returns a string of the snowflake ID
func (f ID) String() string {
	return strconv.FormatInt(int64(f), 10)
}

// Base2 returns a string base2 of the snowflake ID
func (f ID) Base2() string {
	return strconv.FormatInt(int64(f), 2)
}

// Base36 returns a base36 string of the snowflake ID
func (f ID) Base36() string {
	return strconv.FormatInt(int64(f), 36)
}

// Base32 uses the z-base-32 character set but encodes and decodes similar
// to base58, allowing it to create an even smaller result string.
// NOTE: There are many different base32 implementations so becareful when
// doing any interoperation interop with other packages.
func (f ID) Base32() string {

	if f < 32 {
		return string(encodeBase32Map[f])
	}

	b := make([]byte, 0, 12)
	for f >= 32 {
		b = append(b, encodeBase32Map[f%32])
		f /= 32
	}
	b = append(b, encodeBase32Map[f])

	for x, y := 0, len(b)-1; x < y; x, y = x+1, y-1 {
		b[x], b[y] = b[y], b[x]
	}

	return string(b)
}

// ParseBase32 parses a base32 []byte into a snowflake ID
// NOTE: There are many different base32 implementations so becareful when
// doing any interoperation interop with other packages.
func ParseBase32(b []byte) (ID, error) {

	var id int64

	for i := range b {
		if decodeBase32Map[b[i]] == 0xFF {
			return -1, ErrInvalidBase32
		}
		id = id*32 + int64(decodeBase32Map[b[i]])
	}

	return ID(id), nil
}

// Base58 returns a base58 string of the snowflake ID
func (f ID) Base58() string {

	if f < 58 {
		return string(encodeBase58Map[f])
	}

	b := make([]byte, 0, 11)
	for f >= 58 {
		b = append(b, encodeBase58Map[f%58])
		f /= 58
	}
	b = append(b, encodeBase58Map[f])

	for x, y := 0, len(b)-1; x < y; x, y = x+1, y-1 {
		b[x], b[y] = b[y], b[x]
	}

	return string(b)
}

// ParseBase58 parses a base58 []byte into a snowflake ID
func ParseBase58(b []byte) (ID, error) {

	var id int64

	for i := range b {
		if decodeBase58Map[b[i]] == 0xFF {
			return -1, ErrInvalidBase58
		}
		id = id*58 + int64(decodeBase58Map[b[i]])
	}

	return ID(id), nil
}

// Base64 returns a base64 string of the snowflake ID
func (f ID) Base64() string {
	return base64.StdEncoding.EncodeToString(f.Bytes())
}

// Bytes returns a byte slice of the snowflake ID
func (f ID) Bytes() []byte {
	return []byte(f.String())
}

// IntBytes returns an array of bytes of the snowflake ID, encoded as a
// big endian integer.
func (f ID) IntBytes() [8]byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(f))
	return b
}

// Time returns an int64 unix timestamp of the snowflake ID time
func (f ID) Time() int64 {
	return (int64(f) >> timeShift) + Epoch
}

// Node returns an int64 of the snowflake ID node number
func (f ID) Node() int64 {
	return int64(f) & nodeMask >> nodeShift
}

// Step returns an int64 of the snowflake step (or sequence) number
func (f ID) Step() int64 {
	return int64(f) & stepMask
}

// MarshalJSON returns a json byte array string of the snowflake ID.
func (f ID) MarshalJSON() ([]byte, error) {
	buff := make([]byte, 0, 22)
	buff = append(buff, '"')
	buff = strconv.AppendInt(buff, int64(f), 10)
	buff = append(buff, '"')
	return buff, nil
}

// UnmarshalJSON converts a json byte array of a snowflake ID into an ID type.
func (f *ID) UnmarshalJSON(b []byte) error {
	if len(b) < 3 || b[0] != '"' || b[len(b)-1] != '"' {
		return JSONSyntaxError{b}
	}

	i, err := strconv.ParseInt(string(b[1:len(b)-1]), 10, 64)
	if err != nil {
		return err
	}

	*f = ID(i)
	return nil
}
