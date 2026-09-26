package render

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// anchorPrefix opens the line before each round of a sticky body (specs/033-round-anchors). Every value is a decimal
// integer or lowercase hex, so no field can close the comment.
const anchorPrefix = "<!-- loupe-round "

var (
	anchorLine  = regexp.MustCompile(`^<!-- loupe-round ((?:[a-z0-9]+=[0-9a-z]+ )+)sha256=([0-9a-f]{64}) -->$`)
	anchorField = regexp.MustCompile(`^([a-z0-9]+)=([0-9a-z]+)$`)
	hexValue    = regexp.MustCompile(`^[0-9a-f]+$`)
)

// anchor is one parsed anchor line. fields is its text before sha256= exactly as written, which is what the checksum
// covers, so a key a later loupe adds is still checked.
type anchor struct {
	fields string
	sum    string
	n      int
	commit string
	values map[string]string
}

// sealAnchor writes the anchor line for a round: its fields, then a checksum over them and the round's text.
func sealAnchor(fields, text string) string {
	return anchorPrefix + fields + " sha256=" + roundSum(fields, text) + " -->"
}

// roundSum reads line endings as LF and trims blank lines at both ends, as the reader cuts a round out of a body.
func roundSum(fields, text string) string {
	text = strings.Join(trimBlank(strings.Split(normalizeLines(text), "\n")), "\n")
	sum := sha256.Sum256([]byte(fields + "\n" + text))
	return hex.EncodeToString(sum[:])
}

func (a anchor) matches(text string) bool { return a.sum == roundSum(a.fields, text) }

func (a anchor) number(key string) (int, bool) {
	v, ok := a.values[key]
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	return n, err == nil
}

func parseAnchor(line string) (anchor, error) {
	m := anchorLine.FindStringSubmatch(line)
	if m == nil {
		return anchor{}, errors.New("a round anchor is malformed")
	}
	a := anchor{fields: strings.TrimSuffix(m[1], " "), sum: m[2], values: map[string]string{}}
	for i, field := range strings.Split(a.fields, " ") {
		kv := anchorField.FindStringSubmatch(field)
		if _, seen := a.values[kv[1]]; seen {
			return anchor{}, fmt.Errorf("a round anchor repeats %s=", kv[1])
		}
		if i == 0 && kv[1] != "v" {
			return anchor{}, errors.New("a round anchor does not open with v=")
		}
		a.values[kv[1]] = kv[2]
	}
	if v := a.values["v"]; v != "1" {
		return anchor{}, fmt.Errorf("a round anchor has v=%s, which this loupe cannot read", v)
	}
	var ok bool
	if a.n, ok = a.number("n"); !ok || a.n < 1 {
		return anchor{}, errors.New("a round anchor has no round number n=")
	}
	if a.commit = a.values["commit"]; !hexValue.MatchString(a.commit) {
		return anchor{}, errors.New("a round anchor has no commit=")
	}
	return a, nil
}
