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
// O(m*n) space is acceptable here — inputs are small JSON config files.
func computeDiff(a, b []string) []diffOp {
	m, n := len(a), len(b)

	// Build LCS table
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	// Backtrack to produce ops
	var ops []diffOp
	i, j := 0, 0
	for i < m && j < n {
		if a[i] == b[j] {
			ops = append(ops, diffOp{diffEqual, a[i]})
			i++
			j++
		} else if dp[i+1][j] >= dp[i][j+1] {
			ops = append(ops, diffOp{diffDel, a[i]})
			i++
		} else {
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
func formatDiff(ops []diffOp, nameA, nameB string, color bool) string {
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
		b.WriteString(ansiCyan + "--- " + nameA + ansiReset + "\n")
		b.WriteString(ansiCyan + "+++ " + nameB + ansiReset + "\n")
	} else {
		b.WriteString("--- " + nameA + "\n")
		b.WriteString("+++ " + nameB + "\n")
	}

	for _, op := range ops {
		switch op.kind {
		case diffEqual:
			b.WriteString(" " + op.line + "\n")
		case diffDel:
			if color {
				b.WriteString(ansiRed + "-" + op.line + ansiReset + "\n")
			} else {
				b.WriteString("-" + op.line + "\n")
			}
		case diffAdd:
			if color {
				b.WriteString(ansiGreen + "+" + op.line + ansiReset + "\n")
			} else {
				b.WriteString("+" + op.line + "\n")
			}
		}
	}
	return b.String()
}
