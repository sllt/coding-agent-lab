package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func NewID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}

func MustDigest(v any) string {
	d, err := Digest(v)
	if err != nil {
		panic(err)
	}
	return d
}

func FormatCentsUnknown(cost *int64) string {
	if cost == nil {
		return "未知"
	}
	return fmt.Sprintf("%d microusd", *cost)
}
