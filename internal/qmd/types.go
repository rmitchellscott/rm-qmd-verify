package qmd

// HashWithPosition represents a hash value with its position in the source file
type HashWithPosition struct {
	Hash   uint64
	Line   int
	Column int
}
