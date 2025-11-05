package qmd

import (
	"fmt"
	"strconv"
	"unicode"
)

type QMLTokenType int

const (
	QMLKeyword QMLTokenType = iota
	QMLIdentifier
	QMLNumber
	QMLString
	QMLSymbol
	QMLComment
	QMLNewLine
	QMLWhitespace
	QMLEndOfStream
	QMLUnknown
	QMLExtension
)

type QMLExtensionToken struct {
	Type         string
	Hash         uint64
	QuoteChar    rune
	IsIdentifier bool
	IsString     bool
	IsSlot       bool
	SlotName     string
}

type QMLToken struct {
	Type      QMLTokenType
	Value     string
	Extension *QMLExtensionToken
	Line      int
	Column    int
}

type QMLLexer struct {
	stream *StringCharacterTokenizer
}

func NewQMLLexer(input string) *QMLLexer {
	return &QMLLexer{
		stream: NewTokenizer(input),
	}
}

func (l *QMLLexer) NextToken() (*QMLToken, error) {
	line := l.stream.Line
	column := l.stream.Column

	r, ok := l.stream.Peek()
	if !ok {
		return &QMLToken{Type: QMLEndOfStream, Line: line, Column: column}, nil
	}

	switch r {
	case '~':
		nextRune, hasNext := l.stream.PeekOffset(1)
		if hasNext && nextRune == '&' {
			return l.lexHashExtension()
		}
		l.stream.Advance()
		return &QMLToken{Type: QMLSymbol, Value: string(r), Line: line, Column: column}, nil

	case '{', '}', '(', ')', '[', ']', ';', ',', '.', ':', '?', '+', '-', '*', '%', '=', '<', '>', '!', '&', '|', '^':
		l.stream.Advance()
		return &QMLToken{Type: QMLSymbol, Value: string(r), Line: line, Column: column}, nil

	case '\n':
		l.stream.Advance()
		return &QMLToken{Type: QMLNewLine, Value: "\n", Line: line, Column: column}, nil

	case ' ', '\t', '\r':
		ws := l.stream.CollectWhile(func(r rune) bool {
			return r == ' ' || r == '\t' || r == '\r'
		})
		return &QMLToken{Type: QMLWhitespace, Value: ws, Line: line, Column: column}, nil

	case '\'', '"', '`':
		return l.lexString()

	case '/':
		nextRune, hasNext := l.stream.PeekOffset(1)
		if hasNext && (nextRune == '/' || nextRune == '*') {
			return l.lexComment()
		}
		l.stream.Advance()
		return &QMLToken{Type: QMLSymbol, Value: string(r), Line: line, Column: column}, nil

	default:
		if unicode.IsDigit(r) {
			return l.lexNumber()
		}
		if unicode.IsLetter(r) || r == '_' || r == '$' {
			return l.lexIdentifierOrKeyword()
		}
		l.stream.Advance()
		return &QMLToken{Type: QMLUnknown, Value: string(r), Line: line, Column: column}, nil
	}
}

func (l *QMLLexer) lexHashExtension() (*QMLToken, error) {
	line := l.stream.Line
	column := l.stream.Column

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

	hashStr := ""
	for {
		r, ok := l.stream.Peek()
		if !ok {
			return nil, fmt.Errorf("unexpected end of input in hash extension")
		}

		nextRune, hasNext := l.stream.PeekOffset(1)
		if r == '&' && hasNext && nextRune == '~' {
			break
		}

		hashStr += string(r)
		l.stream.Advance()
	}

	l.stream.Advance()
	l.stream.Advance()

	hash, err := strconv.ParseUint(hashStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid hash value: %s", hashStr)
	}

	ext := &QMLExtensionToken{
		Hash: hash,
	}

	if hasQuote {
		ext.IsString = true
		ext.QuoteChar = quoteChar
		ext.Type = "HashedString"
	} else {
		ext.IsIdentifier = true
		ext.Type = "HashedIdentifier"
	}

	return &QMLToken{
		Type:      QMLExtension,
		Extension: ext,
		Line:      line,
		Column:    column,
	}, nil
}

func (l *QMLLexer) lexString() (*QMLToken, error) {
	line := l.stream.Line
	column := l.stream.Column

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

	return &QMLToken{Type: QMLString, Value: str, Line: line, Column: column}, nil
}

func (l *QMLLexer) lexComment() (*QMLToken, error) {
	line := l.stream.Line
	column := l.stream.Column

	l.stream.Advance()
	secondChar, _ := l.stream.Advance()

	if secondChar == '/' {
		comment := l.stream.CollectWhile(func(r rune) bool {
			return r != '\n'
		})
		return &QMLToken{Type: QMLComment, Value: "//" + comment, Line: line, Column: column}, nil
	}

	comment := "/*"
	for {
		r, ok := l.stream.Peek()
		if !ok {
			return nil, fmt.Errorf("unterminated comment")
		}

		comment += string(r)
		l.stream.Advance()

		if r == '*' {
			nextR, hasNext := l.stream.Peek()
			if hasNext && nextR == '/' {
				comment += string(nextR)
				l.stream.Advance()
				break
			}
		}
	}

	return &QMLToken{Type: QMLComment, Value: comment, Line: line, Column: column}, nil
}

func (l *QMLLexer) lexNumber() (*QMLToken, error) {
	line := l.stream.Line
	column := l.stream.Column

	num := l.stream.CollectWhile(func(r rune) bool {
		return unicode.IsDigit(r) || r == '.' || r == 'e' || r == 'E' || r == '+' || r == '-' || r == 'x' || r == 'X'
	})
	return &QMLToken{Type: QMLNumber, Value: num, Line: line, Column: column}, nil
}

func (l *QMLLexer) lexIdentifierOrKeyword() (*QMLToken, error) {
	line := l.stream.Line
	column := l.stream.Column

	ident := l.stream.CollectWhile(func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '$'
	})
	return &QMLToken{Type: QMLIdentifier, Value: ident, Line: line, Column: column}, nil
}

func (l *QMLLexer) Tokenize() ([]*QMLToken, error) {
	tokens := []*QMLToken{}
	for {
		token, err := l.NextToken()
		if err != nil {
			return nil, err
		}
		if token.Type == QMLEndOfStream {
			break
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}
