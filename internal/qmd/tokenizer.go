package qmd

import (
	"unicode/utf8"
)

type StringCharacterTokenizer struct {
	Input    string
	Position int
}

func NewTokenizer(input string) *StringCharacterTokenizer {
	return &StringCharacterTokenizer{
		Input:    input,
		Position: 0,
	}
}

func (t *StringCharacterTokenizer) Peek() (rune, bool) {
	if t.Position >= len(t.Input) {
		return 0, false
	}
	r, _ := utf8.DecodeRuneInString(t.Input[t.Position:])
	if r == utf8.RuneError {
		return 0, false
	}
	return r, true
}

func (t *StringCharacterTokenizer) PeekOffset(offset int) (rune, bool) {
	pos := t.Position
	for i := 0; i < offset; i++ {
		if pos >= len(t.Input) {
			return 0, false
		}
		_, size := utf8.DecodeRuneInString(t.Input[pos:])
		pos += size
	}

	if pos >= len(t.Input) {
		return 0, false
	}

	r, _ := utf8.DecodeRuneInString(t.Input[pos:])
	if r == utf8.RuneError {
		return 0, false
	}
	return r, true
}

func (t *StringCharacterTokenizer) Advance() (rune, bool) {
	r, ok := t.Peek()
	if !ok {
		return 0, false
	}
	_, size := utf8.DecodeRuneInString(t.Input[t.Position:])
	t.Position += size
	return r, true
}

func (t *StringCharacterTokenizer) CollectWhile(condition func(rune) bool) string {
	result := ""
	for {
		r, ok := t.Peek()
		if !ok {
			break
		}
		if condition(r) {
			result += string(r)
			t.Advance()
		} else {
			break
		}
	}
	return result
}

func (t *StringCharacterTokenizer) AtEnd() bool {
	return t.Position >= len(t.Input)
}
