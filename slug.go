package main

import (
	"strings"
	"unicode"
)

func slugify(name string) string {
	var b strings.Builder
	prevHyphen := false
	for _, r := range strings.ToLower(name) {
		switch {
		case unicode.IsLetter(r) || r >= '0' && r <= '9': // ASCII digits only; unicode digits don't belong in filenames
			b.WriteRune(r)
			prevHyphen = false
		case r == ' ' || r == '-':
			if b.Len() > 0 && !prevHyphen {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}
	return strings.TrimRight(b.String(), "-")
}
