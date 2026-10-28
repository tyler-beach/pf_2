package clean

import "crypto/sha256"

func Digester() interface{} {
	return sha256.New()
}
