package encrypt

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/global"
)

const (
	stringGCMVersionPrefix = "enc:gcm:v1:"
	stringGCMKDFContext    = "1panel-x/string-encryption/gcm/v1\x00"
)

func StringDecryptWithBase64(text string) (string, error) {
	decryptItem, err := StringDecrypt(text)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString([]byte(decryptItem)), nil
}
func StringEncryptWithBase64(text string) (string, error) {
	baseItem, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return "", err
	}
	encryptItem, err := StringEncrypt(string(baseItem))
	if err != nil {
		return "", err
	}
	return encryptItem, nil
}

// StringEncryptGCM encrypts text with an authenticated, versioned envelope.
// The non-empty domain is part of both key derivation and GCM additional data,
// so ciphertext cannot be moved between unrelated secret fields.
func StringEncryptGCM(text, domain string) (string, error) {
	key, err := loadStringEncryptionKey()
	if err != nil {
		return "", err
	}
	return StringEncryptGCMWithKey(text, key, domain)
}

// StringDecryptGCM decrypts an envelope produced by StringEncryptGCM.
func StringDecryptGCM(text, domain string) (string, error) {
	key, err := loadStringEncryptionKey()
	if err != nil {
		return "", err
	}
	return StringDecryptGCMWithKey(text, key, domain)
}

// StringEncryptGCMWithKey is the explicit-key form used by migrations and
// tests. It never falls back to plaintext when key material is unavailable.
func StringEncryptGCMWithKey(text, key, domain string) (string, error) {
	if text == "" {
		return "", nil
	}
	derivedKey, err := deriveStringGCMKey(key, domain)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, nonce, []byte(text), []byte(domain))
	payload := append(nonce, sealed...)
	return stringGCMVersionPrefix + base64.RawStdEncoding.EncodeToString(payload), nil
}

// StringDecryptGCMWithKey authenticates before returning plaintext. Invalid
// versions, domains, keys, nonces, or modified ciphertext all fail closed.
func StringDecryptGCMWithKey(text, key, domain string) (string, error) {
	if text == "" {
		return "", nil
	}
	if !strings.HasPrefix(text, stringGCMVersionPrefix) {
		return "", errors.New("unsupported encrypted string version")
	}
	derivedKey, err := deriveStringGCMKey(key, domain)
	if err != nil {
		return "", err
	}
	payload, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(text, stringGCMVersionPrefix))
	if err != nil {
		return "", errors.New("invalid encrypted string payload")
	}
	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(payload) < gcm.NonceSize()+gcm.Overhead() {
		return "", errors.New("invalid encrypted string payload")
	}
	nonce := payload[:gcm.NonceSize()]
	ciphertext := payload[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, []byte(domain))
	if err != nil {
		return "", errors.New("encrypted string authentication failed")
	}
	return string(plain), nil
}

func deriveStringGCMKey(key, domain string) ([]byte, error) {
	if key == "" {
		return nil, errors.New("encryption key is empty")
	}
	if domain == "" {
		return nil, errors.New("encryption domain is empty")
	}
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(stringGCMKDFContext))
	_, _ = mac.Write([]byte(domain))
	return mac.Sum(nil), nil
}

func loadStringEncryptionKey() (string, error) {
	if global.CONF.Base.EncryptKey != "" {
		return global.CONF.Base.EncryptKey, nil
	}
	if global.DB == nil {
		return "", errors.New("encryption key is unavailable")
	}
	var encryptSetting model.Setting
	if err := global.DB.Where("key = ?", "EncryptKey").First(&encryptSetting).Error; err != nil {
		return "", err
	}
	if encryptSetting.Value == "" {
		return "", errors.New("encryption key is empty")
	}
	global.CONF.Base.EncryptKey = encryptSetting.Value
	return encryptSetting.Value, nil
}

func StringEncrypt(text string) (string, error) {
	if len(text) == 0 {
		return "", nil
	}
	if len(global.CONF.Base.EncryptKey) == 0 {
		var encryptSetting model.Setting
		if err := global.DB.Where("key = ?", "EncryptKey").First(&encryptSetting).Error; err != nil {
			return "", err
		}
		global.CONF.Base.EncryptKey = encryptSetting.Value
	}
	key := global.CONF.Base.EncryptKey
	return StringEncryptWithKey(text, key)
}

func StringEncryptWithKey(text, key string) (string, error) {
	if len(text) == 0 || len(key) == 0 {
		return "", nil
	}
	pass := []byte(text)
	xpass, err := aesEncryptWithSalt([]byte(key), pass)
	if err == nil {
		pass64 := base64.StdEncoding.EncodeToString(xpass)
		return pass64, err
	}
	return "", err
}

func StringDecrypt(text string) (string, error) {
	if len(text) == 0 {
		return "", nil
	}
	if len(global.CONF.Base.EncryptKey) == 0 {
		var encryptSetting model.Setting
		if err := global.DB.Where("key = ?", "EncryptKey").First(&encryptSetting).Error; err != nil {
			return "", err
		}
		global.CONF.Base.EncryptKey = encryptSetting.Value
	}
	key := global.CONF.Base.EncryptKey
	return StringDecryptWithKey(text, key)
}

func StringDecryptWithKey(text, key string) (string, error) {
	defer func() {
		if r := recover(); r != nil {
			if global.LOG != nil {
				global.LOG.Errorf("A panic occurred during string decrypt with key, error message: %v", r)
			}
		}
	}()
	if len(text) == 0 {
		return "", nil
	}
	bytesPass, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return "", err
	}
	var tpass []byte
	tpass, err = aesDecryptWithSalt([]byte(key), bytesPass)
	if err == nil {
		result := string(tpass[:])
		return result, err
	}
	return "", err
}

func padding(plaintext []byte, blockSize int) []byte {
	padding := blockSize - len(plaintext)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(plaintext, padtext...)
}

func unPadding(origData []byte) []byte {
	length := len(origData)
	unpadding := int(origData[length-1])
	return origData[:(length - unpadding)]
}

func aesEncryptWithSalt(key, plaintext []byte) ([]byte, error) {
	plaintext = padding(plaintext, aes.BlockSize)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	ciphertext := make([]byte, aes.BlockSize+len(plaintext))
	iv := ciphertext[0:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}
	cbc := cipher.NewCBCEncrypter(block, iv)
	cbc.CryptBlocks(ciphertext[aes.BlockSize:], plaintext)
	return ciphertext, nil
}
func aesDecryptWithSalt(key, ciphertext []byte) ([]byte, error) {
	var block cipher.Block
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < aes.BlockSize {
		return nil, fmt.Errorf("iciphertext too short")
	}
	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]
	cbc := cipher.NewCBCDecrypter(block, iv)
	cbc.CryptBlocks(ciphertext, ciphertext)
	ciphertext = unPadding(ciphertext)
	return ciphertext, nil
}

func ParseRSAPrivateKey(privateKeyPEM string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil || block.Type != "RSA PRIVATE KEY" {
		return nil, errors.New("failed to decode PEM block containing the private key")
	}
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return privateKey, nil
}

func aesDecrypt(ciphertext, key, iv []byte) ([]byte, error) {
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return nil, errors.New("invalid AES key length: must be 16, 24, or 32 bytes")
	}
	if len(iv) != aes.BlockSize {
		return nil, errors.New("invalid IV length: must be 16 bytes")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(ciphertext, ciphertext)
	unpadded, err := pkcs7Unpad(ciphertext)
	if err != nil {
		return nil, err
	}
	return unpadded, nil
}

func pkcs7Unpad(data []byte) ([]byte, error) {
	length := len(data)
	if length == 0 {
		return nil, errors.New("invalid padding size")
	}

	padLength := int(data[length-1])
	if padLength == 0 || padLength > length {
		return nil, errors.New("invalid padding")
	}

	for i := 0; i < padLength; i++ {
		if data[length-1-i] != byte(padLength) {
			return nil, errors.New("invalid padding")
		}
	}

	return data[:length-padLength], nil
}

func DecryptPassword(encryptedData string, privateKey *rsa.PrivateKey) (string, error) {
	parts := strings.Split(encryptedData, ":")
	if len(parts) != 3 {
		return "", errors.New("encrypted data format error")
	}
	keyCipher := parts[0]
	ivBase64 := parts[1]
	ciphertextBase64 := parts[2]

	encryptedAESKey, err := base64.StdEncoding.DecodeString(keyCipher)
	if err != nil {
		return "", errors.New("failed to decode keyCipher")
	}

	aesKey, err := rsa.DecryptPKCS1v15(rand.Reader, privateKey, encryptedAESKey)
	if err != nil {
		return "", errors.New("failed to decode AES Key")
	}

	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextBase64)
	if err != nil {
		return "", errors.New("failed to decrypt the encrypted data")
	}
	iv, err := base64.StdEncoding.DecodeString(ivBase64)
	if err != nil {
		return "", errors.New("failed to decode the IV")
	}

	password, err := aesDecrypt(ciphertext, aesKey, iv)
	if err != nil {
		return "", err
	}
	return string(password), nil
}

func ExportPrivateKeyToPEM(privateKey *rsa.PrivateKey) string {
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})
	return string(privateKeyPEM)
}

func ExportPublicKeyToPEM(publicKey *rsa.PublicKey) (string, error) {
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return "", err
	}
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})
	return string(publicKeyPEM), nil
}
