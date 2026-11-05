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
	return identity{ctimeSec: int64(st.Ctim.Sec), ctimeNsec: int64(st.Ctim.Nsec), ino: st.Ino}, true
}
