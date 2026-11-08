package fakeac

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
)

func encryptCBC(key, plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	out := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(out, plain)

	return out, nil
}

func sha(b []byte) []byte {
	s := sha256.Sum256(b)
	return s[:]
}
