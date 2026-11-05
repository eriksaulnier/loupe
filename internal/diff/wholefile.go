package diff

import (
	"bytes"
	"strconv"
)

// WholeFile writes content as one new-file diff section under the display path, in git's own form. Empty content gets
// the header with no hunk, as git shows for an empty new file.
func WholeFile(path string, content []byte) []byte {
	var b bytes.Buffer
	b.WriteString("diff --git a/" + path + " b/" + path + "\nnew file mode 100644\n")
	if len(content) == 0 {
		return b.Bytes()
	}
	b.WriteString("--- /dev/null\n+++ b/" + path + "\n")
	finalNewline := content[len(content)-1] == '\n'
	body := content
	if finalNewline {
		body = content[:len(content)-1]
	}
	lines := bytes.Split(body, []byte("\n"))
	if len(lines) == 1 {
		b.WriteString("@@ -0,0 +1 @@\n")
	} else {
		b.WriteString("@@ -0,0 +1," + strconv.Itoa(len(lines)) + " @@\n")
	}
	for _, l := range lines {
		b.WriteByte('+')
		b.Write(l)
		b.WriteByte('\n')
	}
	if !finalNewline {
		b.WriteString("\\ No newline at end of file\n")
	}
	return b.Bytes()
}
