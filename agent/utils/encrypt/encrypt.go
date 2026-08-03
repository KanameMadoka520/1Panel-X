package encrypt

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/global"
)

const (
	stringGCMVersionPrefix = "enc:gcm:v1:"
	stringGCMKDFContext    = "1panel-x/string-encryption/gcm/v1\x00"
)

func StringEncryptWithBase64(text string) (string, error) {
	accessKeyItem, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return "", err
	}
	encryptKeyItem, err := StringEncrypt(string(accessKeyItem))
	if err != nil {
		return "", err
	}
	return encryptKeyItem, nil
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

func StringEncryptWithKey(text, key string) (string, error) {
	if len(text) == 0 {
		return "", nil
	}
	if len(key) < 16 {
		for len(key) < 16 {
			key += "u"
		}
	} else {
		key = key[:16]
	}
	pass := []byte(text)
	xpass, err := aesEncryptWithSalt([]byte(key), pass)
	if err == nil {
		pass64 := base64.StdEncoding.EncodeToString(xpass)
		return pass64, err
	}
	return "", err
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

func StringDecryptWithBase64(text string) (string, error) {
	decryptItem, err := StringDecrypt(text)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString([]byte(decryptItem)), nil
}

func StringDecryptWithKey(text, key string) (string, error) {
	defer func() {
		if r := recover(); r != nil {
			global.LOG.Errorf("A panic occurred during string decrypt with key, error message: %v", r)
		}
	}()
	if len(text) == 0 {
		return "", nil
	}
	if len(key) < 16 {
		for len(key) < 16 {
			key += "u"
		}
	} else {
		key = key[:16]
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

func padding(plaintext []byte, blockSize int) []byte {
	padding := blockSize - len(plaintext)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(plaintext, padtext...)
}

func unPadding(origData []byte) ([]byte, error) {
	length := len(origData)
	if length == 0 {
		return nil, fmt.Errorf("invalid padding size")
	}

	unpadding := int(origData[length-1])
	if unpadding == 0 || unpadding > length {
		return nil, fmt.Errorf("invalid padding")
	}

	for i := 0; i < unpadding; i++ {
		if origData[length-1-i] != byte(unpadding) {
			return nil, fmt.Errorf("invalid padding")
		}
	}

	return origData[:(length - unpadding)], nil
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

	unpadded, err := unPadding(ciphertext)
	if err != nil {
		return nil, err
	}
	return unpadded, nil
}
