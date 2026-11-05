package gitx

import (
	"os"
	"syscall"
)

func identityOf(info os.FileInfo) (identity, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return identity{}, false
	}
	return identity{ctimeSec: st.Ctimespec.Sec, ctimeNsec: st.Ctimespec.Nsec, ino: st.Ino}, true
}
