package telemetrysender

import "crypto/sha256"

const salt = "jTfYPbjfq9VlhTDfV9lZEyBI29QYqPqn"

func Anonymize(str string) string {
	hasher := sha256.New()
	hashBytes := hasher.Sum([]byte(salt + str))
	return string(hashBytes)
}
