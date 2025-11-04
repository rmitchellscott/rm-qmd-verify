package qmd

type Parser struct {
	content string
}

func NewParser(content string) *Parser {
	return &Parser{content: content}
}

func (p *Parser) ExtractHashes() ([]uint64, error) {
	return ExtractHashes(p.content)
}
