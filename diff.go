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

import "strings"

type diffKind int

const (
	diffEqual diffKind = iota
	diffAdd
	diffDel
)

type diffOp struct {
	kind diffKind
	line string
}

// computeDiff produces a sequence of diff operations using the LCS algorithm.
// O(m*n) space is acceptable here -- inputs are small JSON config files.
func computeDiff(a, b []string) []diffOp {
	m, n := len(a), len(b)

	// Build LCS table
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			switch {
			case a[i] == b[j]:
				dp[i][j] = dp[i+1][j+1] + 1
			case dp[i+1][j] >= dp[i][j+1]:
				dp[i][j] = dp[i+1][j]
			default:
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	// Backtrack to produce ops
	var ops []diffOp
	i, j := 0, 0
	for i < m && j < n {
		switch {
		case a[i] == b[j]:
			ops = append(ops, diffOp{diffEqual, a[i]})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			ops = append(ops, diffOp{diffDel, a[i]})
			i++
		default:
			ops = append(ops, diffOp{diffAdd, b[j]})
			j++
		}
	}
	for ; i < m; i++ {
		ops = append(ops, diffOp{diffDel, a[i]})
	}
	for ; j < n; j++ {
		ops = append(ops, diffOp{diffAdd, b[j]})
	}
	return ops
}

const (
	ansiReset = "\033[0m"
	ansiRed   = "\033[31m"
	ansiGreen = "\033[32m"
	ansiCyan  = "\033[36m"
)

// formatDiff renders diff ops with --- / +++ headers and +/- line markers.
// Shows full file context (no hunks) since config files are small.
func formatDiff( //nolint:revive // color flag is inherent to output formatting
	ops []diffOp, nameA, nameB string, color bool,
) string {
	hasChanges := false
	for _, op := range ops {
		if op.kind != diffEqual {
			hasChanges = true
			break
		}
	}
	if !hasChanges {
		return ""
	}

	var b strings.Builder
	if color {
		_, _ = b.WriteString(ansiCyan + "--- " + nameA + ansiReset + "\n")
		_, _ = b.WriteString(ansiCyan + "+++ " + nameB + ansiReset + "\n")
	} else {
		_, _ = b.WriteString("--- " + nameA + "\n")
		_, _ = b.WriteString("+++ " + nameB + "\n")
	}

	for _, op := range ops {
		switch op.kind {
		case diffEqual:
			_, _ = b.WriteString(" " + op.line + "\n")
		case diffDel:
			if color {
				_, _ = b.WriteString(ansiRed + "-" + op.line + ansiReset + "\n")
			} else {
				_, _ = b.WriteString("-" + op.line + "\n")
			}
		case diffAdd:
			if color {
				_, _ = b.WriteString(ansiGreen + "+" + op.line + ansiReset + "\n")
			} else {
				_, _ = b.WriteString("+" + op.line + "\n")
			}
		default:
		}
	}
	return b.String()
}
