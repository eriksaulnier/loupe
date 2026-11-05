//go:build !darwin && !linux

package gitx

import "os"

// identityOf reports no identity where loupe does not read one, so a filtered file there never counts as untouched.
func identityOf(os.FileInfo) (identity, bool) { return identity{}, false }
