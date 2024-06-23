package telemetrysender

import (
	"crypto/sha256"
	"fmt"
)

const salt = "jTfYPbjfq9VlhTDfV9lZEyBI29QYqPqn"

func Anonymize(str string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(salt+str)))
}
