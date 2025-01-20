// Code generated by ./cmd/ch-gen-col, DO NOT EDIT.

package proto

// ColEnum8 represents Enum8 column.
type ColEnum8 []Enum8

// Compile-time assertions for ColEnum8.
var (
	_ ColInput  = ColEnum8{}
	_ ColResult = (*ColEnum8)(nil)
	_ Column    = (*ColEnum8)(nil)
)

// Rows returns count of rows in column.
func (c ColEnum8) Rows() int {
	return len(c)
}

// Reset resets data in row, preserving capacity for efficiency.
func (c *ColEnum8) Reset() {
	*c = (*c)[:0]
}

// Type returns ColumnType of Enum8.
func (ColEnum8) Type() ColumnType {
	return ColumnTypeEnum8
}

// Row returns i-th row of column.
func (c ColEnum8) Row(i int) Enum8 {
	return c[i]
}

// Append Enum8 to column.
func (c *ColEnum8) Append(v Enum8) {
	*c = append(*c, v)
}

// Append Enum8 slice to column.
func (c *ColEnum8) AppendArr(vs []Enum8) {
	*c = append(*c, vs...)
}

// LowCardinality returns LowCardinality for Enum8 .
func (c *ColEnum8) LowCardinality() *ColLowCardinality[Enum8] {
	return &ColLowCardinality[Enum8]{
		index: c,
	}
}

// Array is helper that creates Array of Enum8.
func (c *ColEnum8) Array() *ColArr[Enum8] {
	return &ColArr[Enum8]{
		Data: c,
	}
}

// Nullable is helper that creates Nullable(Enum8).
func (c *ColEnum8) Nullable() *ColNullable[Enum8] {
	return &ColNullable[Enum8]{
		Values: c,
	}
}

// NewArrEnum8 returns new Array(Enum8).
func NewArrEnum8() *ColArr[Enum8] {
	return &ColArr[Enum8]{
		Data: new(ColEnum8),
	}
}
