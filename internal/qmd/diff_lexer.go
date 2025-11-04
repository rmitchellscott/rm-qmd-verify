package qmd

import (
	"fmt"
	"strconv"
	"unicode"
)

type DiffTokenType int

const (
	DiffKeyword DiffTokenType = iota
	DiffIdentifier
	DiffString
	DiffSymbol
	DiffComment
	DiffNewLine
	DiffWhitespace
	DiffEndOfStream
	DiffUnknown
	DiffHashedValue
	DiffQMLCode
)

type HashedValue struct {
	Hash         uint64
	QuoteChar    rune
	IsIdentifier bool
	IsString     bool
}

type DiffToken struct {
	Type        DiffTokenType
	Value       string
	HashedValue *HashedValue
	QMLCode     []*QMLToken
}

type DiffLexer struct {
	stream *StringCharacterTokenizer
}

func NewDiffLexer(input string) *DiffLexer {
	return &DiffLexer{
		stream: NewTokenizer(input),
	}
}

func (l *DiffLexer) NextToken() (*DiffToken, error) {
	r, ok := l.stream.Peek()
	if !ok {
		return &DiffToken{Type: DiffEndOfStream}, nil
	}

	switch r {
	case '[':
		nextRune, hasNext := l.stream.PeekOffset(1)
		if hasNext && nextRune == '[' {
			return l.lexHashedValue()
		}
		l.stream.Advance()
		return &DiffToken{Type: DiffSymbol, Value: string(r)}, nil

	case '{':
		return l.lexBracedQMLBlock()

	case '\n':
		l.stream.Advance()
		return &DiffToken{Type: DiffNewLine, Value: "\n"}, nil

	case ' ', '\t', '\r':
		ws := l.stream.CollectWhile(func(r rune) bool {
			return r == ' ' || r == '\t' || r == '\r'
		})
		return &DiffToken{Type: DiffWhitespace, Value: ws}, nil

	case ';':
		comment := l.stream.CollectWhile(func(r rune) bool {
			return r != '\n'
		})
		return &DiffToken{Type: DiffComment, Value: comment}, nil

	case '\'', '"', '`':
		return l.lexString()

	default:
		if unicode.IsLetter(r) || r == '_' {
			return l.lexIdentifierOrKeyword()
		}
		l.stream.Advance()
		return &DiffToken{Type: DiffSymbol, Value: string(r)}, nil
	}
}

func (l *DiffLexer) lexHashedValue() (*DiffToken, error) {
	l.stream.Advance()
	l.stream.Advance()

	var quoteChar rune
	hasQuote := false

	r, ok := l.stream.Peek()
	if ok && (r == '\'' || r == '"' || r == '`') {
		quoteChar = r
		hasQuote = true
		l.stream.Advance()
	}

	hashStr := l.stream.CollectWhile(func(r rune) bool {
		return unicode.IsDigit(r)
	})

	if hashStr == "" {
		return nil, fmt.Errorf("invalid hash: no digits found")
	}

	firstBracket, ok := l.stream.Peek()
	if !ok || firstBracket != ']' {
		return nil, fmt.Errorf("invalid hash: expected ']'")
	}
	l.stream.Advance()

	secondBracket, ok := l.stream.Peek()
	if !ok || secondBracket != ']' {
		return nil, fmt.Errorf("invalid hash: expected second ']'")
	}
	l.stream.Advance()

	hash, err := strconv.ParseUint(hashStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid hash value: %s", hashStr)
	}

	hv := &HashedValue{
		Hash: hash,
	}

	if hasQuote {
		hv.IsString = true
		hv.QuoteChar = quoteChar
	} else {
		hv.IsIdentifier = true
	}

	return &DiffToken{
		Type:        DiffHashedValue,
		HashedValue: hv,
	}, nil
}

func (l *DiffLexer) lexBracedQMLBlock() (*DiffToken, error) {
	l.stream.Advance()

	qmlStart := l.stream.Position
	depth := 1

	for depth > 0 {
		r, ok := l.stream.Peek()
		if !ok {
			return nil, fmt.Errorf("unterminated QML block")
		}

		if r == '{' {
			depth++
		} else if r == '}' {
			depth--
		}

		l.stream.Advance()

		if depth == 0 {
			break
		}
	}

	qmlEnd := l.stream.Position - 1
	qmlContent := l.stream.Input[qmlStart:qmlEnd]

	qmlLexer := NewQMLLexer(qmlContent)
	qmlTokens, err := qmlLexer.Tokenize()
	if err != nil {
		return nil, fmt.Errorf("failed to lex QML code: %w", err)
	}

	return &DiffToken{
		Type:    DiffQMLCode,
		QMLCode: qmlTokens,
	}, nil
}

func (l *DiffLexer) lexString() (*DiffToken, error) {
	quoteChar, _ := l.stream.Advance()
	str := string(quoteChar)

	for {
		r, ok := l.stream.Peek()
		if !ok {
			return nil, fmt.Errorf("unterminated string")
		}

		if r == quoteChar {
			str += string(r)
			l.stream.Advance()
			break
		}

		if r == '\\' {
			str += string(r)
			l.stream.Advance()
			nextR, ok := l.stream.Peek()
			if ok {
				str += string(nextR)
				l.stream.Advance()
			}
			continue
		}

		str += string(r)
		l.stream.Advance()
	}

	return &DiffToken{Type: DiffString, Value: str}, nil
}

func (l *DiffLexer) lexIdentifierOrKeyword() (*DiffToken, error) {
	ident := l.stream.CollectWhile(func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
	})

	if ident == "STREAM" {
		l.stream.CollectWhile(func(r rune) bool {
			return unicode.IsSpace(r) && r != '\n'
		})

		return l.lexStreamQMLBlock()
	}

	return &DiffToken{Type: DiffIdentifier, Value: ident}, nil
}

func (l *DiffLexer) lexStreamQMLBlock() (*DiffToken, error) {
	qmlStart := l.stream.Position

	initialChar, ok := l.stream.Peek()
	if !ok {
		return nil, fmt.Errorf("expected delimiter after STREAM")
	}
	l.stream.Advance()

	for {
		r, ok := l.stream.Peek()
		if !ok {
			return nil, fmt.Errorf("unterminated STREAM block")
		}

		if r == initialChar {
			qmlEnd := l.stream.Position
			l.stream.Advance()

			qmlContent := l.stream.Input[qmlStart+1 : qmlEnd]

			qmlLexer := NewQMLLexer(qmlContent)
			qmlTokens, err := qmlLexer.Tokenize()
			if err != nil {
				return nil, fmt.Errorf("failed to lex STREAM QML code: %w", err)
			}

			return &DiffToken{
				Type:    DiffQMLCode,
				QMLCode: qmlTokens,
			}, nil
		}

		l.stream.Advance()
	}
}

func (l *DiffLexer) Tokenize() ([]*DiffToken, error) {
	tokens := []*DiffToken{}
	for {
		token, err := l.NextToken()
		if err != nil {
			return nil, err
		}
		if token.Type == DiffEndOfStream {
			break
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}
