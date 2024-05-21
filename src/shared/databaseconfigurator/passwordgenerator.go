package databaseconfigurator

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/aidarkhanov/nanoid"
	"github.com/otterize/intents-operator/src/shared/errors"
	"golang.org/x/crypto/pbkdf2"
)

const (
	DefaultCredentialsAlphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	DefaultCredentialsLen      = 16
)

func GenerateRandomPassword() (string, error) {
	password, err := nanoid.Generate(DefaultCredentialsAlphabet, DefaultCredentialsLen)
	if err != nil {
		return "", errors.Wrap(err)
	}
	salt, err := nanoid.Generate(DefaultCredentialsAlphabet, 8)
	if err != nil {
		return "", errors.Wrap(err)
	}

	dk := pbkdf2.Key([]byte(password), []byte(salt), 2048, 16, sha256.New)
	return hex.EncodeToString(dk), nil
}
