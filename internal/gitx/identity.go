package gitx

// identity is the part of a file's stat that Git's index records beyond size and mtime: the inode change time and
// the inode number.
type identity struct {
	ctimeSec, ctimeNsec int64
	ino                 uint64
}
