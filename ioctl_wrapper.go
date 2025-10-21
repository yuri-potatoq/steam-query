package main

/*
#include <sys/ioctl.h>
int get_win_col(int *row) {
	struct winsize wsz;
	int n = ioctl(0, TIOCGWINSZ, &wsz);
	if (n == 0) {
		*row = wsz.ws_col;
	}
	return n;
}
*/
import "C"
import (
	"errors"
)

const TIOCGWINSZ = 0x5413

func GetTermSize() (int, error) {
	row := C.int(0)
	if n := C.get_win_col(&row); n == -1 {
		return 0, errors.New("can't get windows row ")
	}
	return int(row), nil
}

/*
 * https://cs.opensource.google/go/x/sys/+/refs/tags/v0.37.0:unix/ioctl_unsigned.go;l=59
 * https://cs.opensource.google/go/x/sys/+/master:unix/syscall_hurd.go;l=24?q=ioctlPtr&ss=go%2Fx%2Fsys
 * https://man7.org/linux/man-pages/man2/TIOCSWINSZ.2const.html
 */
