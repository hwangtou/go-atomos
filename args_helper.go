package atomos

import (
	"fmt"
	"strings"
)

// argIssue records a single problem found while parsing an args slice.
// Shared by baseAtomosHelper and taskHelper so the two parsers don't each
// maintain their own near-identical "invalid arg" struct.
type argIssue struct {
	idx    int
	reason string
}

// argIssueList collects arg parse problems and renders them into a single
// human-readable reason string. Both baseAtomosHelper.getError and
// taskHelper.getErrorReason delegate here, keeping the wording consistent.
type argIssueList struct {
	invalid  []argIssue
	conflict []argIssue
}

func (l *argIssueList) addInvalid(idx int, reason string) {
	l.invalid = append(l.invalid, argIssue{idx: idx, reason: reason})
}

func (l *argIssueList) addConflict(idx int, reason string) {
	l.conflict = append(l.conflict, argIssue{idx: idx, reason: reason})
}

func (l *argIssueList) hasAny() bool {
	return len(l.invalid) > 0 || len(l.conflict) > 0
}

// render builds the reason string. invalidHeader / conflictHeader let each
// helper keep its preferred section label ("Invalid Arguments" etc.) while
// sharing the per-entry formatting.
func (l *argIssueList) render(invalidHeader, conflictHeader string) string {
	var builder strings.Builder
	if len(l.invalid) > 0 {
		builder.WriteString(invalidHeader)
		for _, ia := range l.invalid {
			builder.WriteString(fmt.Sprintf("\nAt index %d is invalid: %s.", ia.idx, ia.reason))
		}
		builder.WriteString("\n")
	}
	if len(l.conflict) > 0 {
		builder.WriteString(conflictHeader)
		for _, ca := range l.conflict {
			builder.WriteString(fmt.Sprintf("\nAt index %d is conflict: %s.", ca.idx, ca.reason))
		}
		builder.WriteString("\n")
	}
	return builder.String()
}
