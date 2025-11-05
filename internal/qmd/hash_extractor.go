package qmd

type HashWithPosition struct {
	Hash   uint64
	Line   int
	Column int
}

func ExtractHashesFromTokens(tokens []*DiffToken) []HashWithPosition {
	hashMap := make(map[uint64]*HashWithPosition)

	for _, token := range tokens {
		if token.Type == DiffHashedValue && token.HashedValue != nil {
			hash := token.HashedValue.Hash
			if _, exists := hashMap[hash]; !exists {
				hashMap[hash] = &HashWithPosition{
					Hash:   hash,
					Line:   token.Line,
					Column: token.Column,
				}
			}
		}

		if token.Type == DiffQMLCode && token.QMLCode != nil {
			extractHashesFromQMLTokens(token.QMLCode, hashMap)
		}
	}

	hashes := make([]HashWithPosition, 0, len(hashMap))
	for _, hashPos := range hashMap {
		hashes = append(hashes, *hashPos)
	}

	return hashes
}

func extractHashesFromQMLTokens(tokens []*QMLToken, hashMap map[uint64]*HashWithPosition) {
	for _, token := range tokens {
		if token.Type == QMLExtension && token.Extension != nil {
			hash := token.Extension.Hash
			if _, exists := hashMap[hash]; !exists {
				hashMap[hash] = &HashWithPosition{
					Hash:   hash,
					Line:   token.Line,
					Column: token.Column,
				}
			}
		}
	}
}

func ExtractHashes(content string) ([]HashWithPosition, error) {
	lexer := NewDiffLexer(content)
	tokens, err := lexer.Tokenize()
	if err != nil {
		return nil, err
	}

	return ExtractHashesFromTokens(tokens), nil
}
