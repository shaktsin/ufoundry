package projects

import (
	"fmt"
	"strings"
)

const (
	maxDiffInput = 512 << 10 // files larger than this are summarised, not diffed
	maxDiffBytes = 128 << 10 // the unified diff itself is capped
	diffContext  = 3
)

// Unified returns a unified diff of before → after, plus the number of added
// and removed lines. Very large inputs are reported without a body.
func Unified(path, before, after string) (diff string, additions, deletions int, truncated bool) {
	if before == after {
		return "", 0, 0, false
	}
	if len(before) > maxDiffInput || len(after) > maxDiffInput {
		a, d := countLines(after), countLines(before)
		return "", a, d, true
	}
	ba, aa := splitLines(before), splitLines(after)
	ops := diffLines(ba, aa)
	for _, op := range ops {
		switch op.kind {
		case opAdd:
			additions++
		case opDel:
			deletions++
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "--- a/%s\n+++ b/%s\n", path, path)
	writeHunks(&b, ops)
	out := b.String()
	if len(out) > maxDiffBytes {
		out = out[:maxDiffBytes] + "\n… diff truncated\n"
		truncated = true
	}
	return out, additions, deletions, truncated
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.SplitAfter(s, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines
}

type opKind int

const (
	opKeep opKind = iota
	opAdd
	opDel
)

type diffOp struct {
	kind opKind
	text string
	// line numbers, 1-based, in the file the line comes from
	aLine, bLine int
}

// diffLines is a straightforward LCS diff. Files here are source files, so the
// quadratic table is fine; huge inputs are rejected before we get this far.
func diffLines(a, b []string) []diffOp {
	n, m := len(a), len(b)
	// Trim the common prefix and suffix first: most edits are local.
	start := 0
	for start < n && start < m && a[start] == b[start] {
		start++
	}
	endA, endB := n, m
	for endA > start && endB > start && a[endA-1] == b[endB-1] {
		endA--
		endB--
	}
	midA, midB := a[start:endA], b[start:endB]
	lcs := make([][]int, len(midA)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(midB)+1)
	}
	for i := len(midA) - 1; i >= 0; i-- {
		for j := len(midB) - 1; j >= 0; j-- {
			if midA[i] == midB[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	var ops []diffOp
	for i := 0; i < start; i++ {
		ops = append(ops, diffOp{kind: opKeep, text: a[i], aLine: i + 1, bLine: i + 1})
	}
	i, j := 0, 0
	for i < len(midA) && j < len(midB) {
		switch {
		case midA[i] == midB[j]:
			ops = append(ops, diffOp{kind: opKeep, text: midA[i], aLine: start + i + 1, bLine: start + j + 1})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, diffOp{kind: opDel, text: midA[i], aLine: start + i + 1})
			i++
		default:
			ops = append(ops, diffOp{kind: opAdd, text: midB[j], bLine: start + j + 1})
			j++
		}
	}
	for ; i < len(midA); i++ {
		ops = append(ops, diffOp{kind: opDel, text: midA[i], aLine: start + i + 1})
	}
	for ; j < len(midB); j++ {
		ops = append(ops, diffOp{kind: opAdd, text: midB[j], bLine: start + j + 1})
	}
	for k := 0; k < n-endA; k++ {
		ops = append(ops, diffOp{kind: opKeep, text: a[endA+k], aLine: endA + k + 1, bLine: endB + k + 1})
	}
	return ops
}

// writeHunks prints changed regions with a few lines of context around them.
func writeHunks(b *strings.Builder, ops []diffOp) {
	changed := func(i int) bool { return ops[i].kind != opKeep }
	i := 0
	for i < len(ops) {
		if !changed(i) {
			i++
			continue
		}
		start := i - diffContext
		if start < 0 {
			start = 0
		}
		end := i
		for end < len(ops) {
			if changed(end) {
				end++
				continue
			}
			// Look ahead: keep the hunk going if another change is close by.
			next := end
			for next < len(ops) && next < end+2*diffContext && !changed(next) {
				next++
			}
			if next < len(ops) && changed(next) {
				end = next
				continue
			}
			break
		}
		stop := end + diffContext
		if stop > len(ops) {
			stop = len(ops)
		}
		aStart, bStart, aCount, bCount := 0, 0, 0, 0
		for _, op := range ops[start:stop] {
			if op.kind != opAdd {
				if aStart == 0 {
					aStart = op.aLine
				}
				aCount++
			}
			if op.kind != opDel {
				if bStart == 0 {
					bStart = op.bLine
				}
				bCount++
			}
		}
		fmt.Fprintf(b, "@@ -%d,%d +%d,%d @@\n", aStart, aCount, bStart, bCount)
		for _, op := range ops[start:stop] {
			prefix := " "
			switch op.kind {
			case opAdd:
				prefix = "+"
			case opDel:
				prefix = "-"
			}
			line := op.text
			if !strings.HasSuffix(line, "\n") {
				line += "\n\\ No newline at end of file\n"
			}
			b.WriteString(prefix + line)
		}
		i = stop
	}
}
