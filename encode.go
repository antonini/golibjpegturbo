package golibjpegturbo

/*
#include <stddef.h>
#include <stdio.h>
#include <stdlib.h>
#include <jpeglib.h>
typedef unsigned char *PUCHAR;

void error_panic(j_common_ptr cinfo);

typedef struct {
unsigned char *buf;
unsigned long buf_size;
} mem_helper;

mem_helper *alloc_mem_helper();
*/
import "C"

import (
	"fmt"
	"image"
	"io"
	"unsafe"
)

// DefaultQuality is the default quality encoding parameter.
const DefaultQuality = 75

// Options are the encoding parameters.
// Quality ranges from 1 to 100 inclusive, higher is better.
type Options struct {
	Quality int
}

// Encode writes the Image m to w in JPEG 4:2:0 baseline format with the given
// options. Default parameters are used if a nil *Options is passed.
func Encode(w io.Writer, m image.Image, o *Options) (err error) {
	defer func() {
		if r := recover(); r != nil {
			var ok bool
			err, ok = r.(error)
			if !ok {
				err = fmt.Errorf("JPEG error: %v", r)
			}
		}
	}()

	b := m.Bounds()
	dx := b.Dx()
	dy := b.Dy()
	if dx <= 0 || dy <= 0 {
		return fmt.Errorf("image with invalid size, dx: %d, dy: %d (both must be > 0)", dx, dy)
	}

	quality := 75
	if o != nil {
		quality = o.Quality
	}

	cinfoSize := C.size_t(unsafe.Sizeof(C.struct_jpeg_compress_struct{}))

	cinfo := (*C.struct_jpeg_compress_struct)(C.malloc(cinfoSize))
	defer C.free(unsafe.Pointer(cinfo))

	cinfoErrSize := C.size_t(unsafe.Sizeof(C.struct_jpeg_error_mgr{}))
	cinfo.err = (*C.struct_jpeg_error_mgr)(C.malloc(cinfoErrSize))
	defer C.free(unsafe.Pointer(cinfo.err))

	C.jpeg_std_error(cinfo.err)
	cinfo.err.error_exit = (*[0]byte)(C.error_panic)

	memHelper := C.alloc_mem_helper()
	C.jpeg_CreateCompress(cinfo, C.JPEG_LIB_VERSION, cinfoSize)
	C.jpeg_mem_dest(cinfo, &memHelper.buf, &memHelper.buf_size)

	nBytes := dx * 3 // for a line, 3 bytes per pixel
	cinfo.image_width = C.JDIMENSION(dx)
	cinfo.image_height = C.JDIMENSION(dy)

	gray, isGray := m.(*image.Gray)
	rgba, isRgba := m.(*image.RGBA)

	cinfo.input_components = 3
	cinfo.in_color_space = C.JCS_RGB

	if isGray {
		nBytes = dx
		cinfo.input_components = 1
		cinfo.in_color_space = C.JCS_GRAYSCALE
	}
	// Note: for more speed could try to go directly to JCS_EXT_RGBA but not
	// sure if libjpeg matches Go and treats JCS_EXT_RGBA as alpha-premultipled
	if isRgba {
		nBytes = dx * 3
		cinfo.input_components = 3
		cinfo.in_color_space = C.JCS_RGB
	}

	C.jpeg_set_defaults(cinfo)
	C.jpeg_set_quality(cinfo, C.int(quality), C.TRUE)
	C.jpeg_start_compress(cinfo, C.TRUE)

	bufBytes := C.malloc(C.size_t(nBytes))
	rowPtr := C.JSAMPROW(bufBytes)
	buf := sliceFromCBytes(bufBytes, nBytes)

	if isGray {
		for y := 0; y < dy; y++ {
			off := y * gray.Stride
			copy(buf[:], gray.Pix[off:off+nBytes])
			C.jpeg_write_scanlines(cinfo, &rowPtr, 1)
		}
	} else if isRgba {
		for y := 0; y < dy; y++ {
			off := y * rgba.Stride
			p := rgba.Pix[off:]
			dstOff := 0
			srcOff := 0
			for x := 0; x < dx; x++ {
				buf[dstOff] = p[srcOff]
				dstOff++
				srcOff++
				buf[dstOff] = p[srcOff]
				dstOff++
				srcOff++
				buf[dstOff] = p[srcOff]
				dstOff++
				srcOff += 2
			}

			C.jpeg_write_scanlines(cinfo, &rowPtr, 1)
		}
	} else {
		for y := 0; y < dy; y++ {
			off := 0
			for x := 0; x < dx; x++ {
				r, g, b, _ := m.At(x, y).RGBA()
				buf[off] = byte(r >> 8)
				off++
				buf[off] = byte(g >> 8)
				off++
				buf[off] = byte(b >> 8)
				off++
			}

			C.jpeg_write_scanlines(cinfo, &rowPtr, 1)
		}
	}

	C.jpeg_finish_compress(cinfo)
	C.jpeg_destroy_compress(cinfo)

	outBs := C.GoBytes(unsafe.Pointer(memHelper.buf), C.int(memHelper.buf_size))
	w.Write(outBs)
	C.free(unsafe.Pointer(memHelper.buf))
	C.free(bufBytes)
	C.free(unsafe.Pointer(memHelper))
	return nil
}
