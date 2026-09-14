package diff

import (
	"fmt"
	"strings"
	"testing"
)

// syntheticDiff gives every file two hunks that each replace one line with three.
func syntheticDiff(files int) []byte {
	var b strings.Builder
	for i := range files {
		name := fmt.Sprintf("pkg%02d/file%03d.go", i%20, i)
		fmt.Fprintf(&b, "diff --git a/%s b/%s\nindex 1111111..2222222 100644\n--- a/%s\n+++ b/%s\n", name, name, name, name)
		for _, start := range []int{10, 200} {
			fmt.Fprintf(&b, "@@ -%d,18 +%d,20 @@ func f%d() {\n", start, start, i)
			for j := range 18 {
				if j == 6 {
					fmt.Fprintf(&b, "-\told := %d\n+\tnew := %d\n+\tnewer := %d\n+\tnewest := %d\n", j, j, j, j)
					continue
				}
				fmt.Fprintf(&b, " \tline(%d)\n", start+j)
			}
		}
	}
	return []byte(b.String())
}

func BenchmarkParseAndLocate(b *testing.B) {
	if testing.Short() {
		b.Skip("performance check")
	}
	data := syntheticDiff(500)
	b.ReportAllocs()
	for b.Loop() {
		d, err := Parse(data)
		if err != nil {
			b.Fatal(err)
		}
		if _, _, err := d.HunkFor("pkg10/file250.go", "RIGHT", 207); err != nil {
			b.Fatal(err)
		}
	}
}
