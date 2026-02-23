// Copyright © 2026 Michael Shields
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
			_, _ = b.WriteRune(r)
			prevHyphen = false
		case r == ' ' || r == '-':
			if b.Len() > 0 && !prevHyphen {
				_ = b.WriteByte('-')
				prevHyphen = true
			}
		default:
		}
	}
	return strings.TrimRight(b.String(), "-")
}
