// Package midea implements the local (LAN, protocol V3 "8370") and cloud
// protocol of Midea air conditioners, in particular the Midea PortaSplit.
//
// The implementation follows Klimacontrol (github.com/mlafx/Klimacontrol),
// msmart-ng, midea-beautiful-air and midea_ac_lan.
//
// Crypto limits of the protocol (given by Midea, documented here only):
//   - V2 packets and discovery use AES-128-ECB with a fixed, publicly known key
//     and an MD5 "signature" with the same fixed key. That only protects
//     against transmission errors, never against forgery: a discovery answer
//     must never be trusted before a handshake with the stored token worked.
//   - V3 uses a handshake with token/key from the cloud, then AES-CBC with a
//     zero IV and SHA256 over the plain text. Token and key are the secret.
package midea

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5" //nolint:gosec // mandated by the protocol
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

var (
	signKey = []byte("xhdiwjnchekd4d512chdjx5d8e4c394D2D7S")
	encKey  = md5Sum(signKey)
)

// ErrCrypto is returned for malformed cipher text or padding.
var ErrCrypto = errors.New("midea: crypto error")

func md5Sum(b []byte) []byte {
	s := md5.Sum(b) //nolint:gosec // protocol
	return s[:]
}

func sha256Sum(b []byte) []byte {
	s := sha256.Sum256(b)
	return s[:]
}

// sign is the MD5 "signature" of V2 packets: MD5(packet + signKey).
func sign(b []byte) []byte {
	buf := make([]byte, 0, len(b)+len(signKey))
	buf = append(buf, b...)
	buf = append(buf, signKey...)

	return md5Sum(buf)
}

func pkcs7Pad(b []byte) []byte {
	n := aes.BlockSize - len(b)%aes.BlockSize
	return append(append([]byte{}, b...), bytes.Repeat([]byte{byte(n)}, n)...)
}

func pkcs7Unpad(b []byte) ([]byte, error) {
	if len(b) == 0 || len(b)%aes.BlockSize != 0 {
		return nil, ErrCrypto
	}

	n := int(b[len(b)-1])
	if n < 1 || n > aes.BlockSize || n > len(b) {
		return nil, ErrCrypto
	}

	for _, c := range b[len(b)-n:] {
		if int(c) != n {
			return nil, ErrCrypto
		}
	}

	return b[:len(b)-n], nil
}

// ecb runs AES-ECB over whole blocks.
func ecb(key, data []byte, encrypt bool) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	if len(data)%aes.BlockSize != 0 {
		return nil, ErrCrypto
	}

	out := make([]byte, len(data))
	for i := 0; i < len(data); i += aes.BlockSize {
		if encrypt {
			block.Encrypt(out[i:i+aes.BlockSize], data[i:i+aes.BlockSize])
		} else {
			block.Decrypt(out[i:i+aes.BlockSize], data[i:i+aes.BlockSize])
		}
	}

	return out, nil
}

// EncryptECB encrypts with AES-ECB and PKCS7 padding.
func EncryptECB(key, data []byte) ([]byte, error) { return ecb(key, pkcs7Pad(data), true) }

// DecryptECB decrypts AES-ECB and removes PKCS7 padding.
func DecryptECB(key, data []byte) ([]byte, error) {
	plain, err := ecb(key, data, false)
	if err != nil {
		return nil, err
	}

	return pkcs7Unpad(plain)
}

func cbc(key, iv, data []byte, encrypt bool) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	if len(data)%aes.BlockSize != 0 || len(iv) != aes.BlockSize {
		return nil, ErrCrypto
	}

	out := make([]byte, len(data))
	if encrypt {
		cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, data)
	} else {
		cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, data)
	}

	return out, nil
}

var zeroIV = make([]byte, aes.BlockSize)

// encryptCBCRaw is AES-CBC with a zero IV and no padding (V3 transport).
func encryptCBCRaw(key, data []byte) ([]byte, error) { return cbc(key, zeroIV, data, true) }

func decryptCBCRaw(key, data []byte) ([]byte, error) { return cbc(key, zeroIV, data, false) }

// EncryptCBC is AES-CBC with an IV and PKCS7 padding (cloud session keys).
func EncryptCBC(key, iv, data []byte) ([]byte, error) { return cbc(key, iv, pkcs7Pad(data), true) }

// DecryptCBC is AES-CBC with an IV and PKCS7 padding.
func DecryptCBC(key, iv, data []byte) ([]byte, error) {
	plain, err := cbc(key, iv, data, false)
	if err != nil {
		return nil, err
	}

	return pkcs7Unpad(plain)
}

func hexs(b []byte) string { return hex.EncodeToString(b) }
