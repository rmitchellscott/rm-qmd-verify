package qmd

func ExtractHashesFromTokens(tokens []*DiffToken) []uint64 {
	hashSet := make(map[uint64]bool)

	for _, token := range tokens {
		if token.Type == DiffHashedValue && token.HashedValue != nil {
			hashSet[token.HashedValue.Hash] = true
		}

		if token.Type == DiffQMLCode && token.QMLCode != nil {
			extractHashesFromQMLTokens(token.QMLCode, hashSet)
		}
	}

	hashes := make([]uint64, 0, len(hashSet))
	for hash := range hashSet {
		hashes = append(hashes, hash)
	}

	return hashes
}

func extractHashesFromQMLTokens(tokens []*QMLToken, hashSet map[uint64]bool) {
	for _, token := range tokens {
		if token.Type == QMLExtension && token.Extension != nil {
			hashSet[token.Extension.Hash] = true
		}
	}
}

func ExtractHashes(content string) ([]uint64, error) {
	lexer := NewDiffLexer(content)
	tokens, err := lexer.Tokenize()
	if err != nil {
		return nil, err
	}

	return ExtractHashesFromTokens(tokens), nil
}
