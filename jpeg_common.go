package libjpeg

/*
#cgo LDFLAGS: -ljpeg

#include <stdlib.h>
#include <stdio.h>
#include <jpeglib.h>

void goPanic(char *);
*/
import "C"

//export goPanic
func goPanic(msg *C.char) {
	panic(C.GoString(msg))
}
