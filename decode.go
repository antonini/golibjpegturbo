/*
Package golibjpegturbo is the fastest way to decode and encode JPEG images in Go.

We achieve this via cgo bindings to http://libjpeg-turbo.virtualgl.org library.

The exact speed depends on the image and CPU. On Mac Book Pro, compared
to image/jpeg standard library, golibjpegturbo is:

* 6x faster when decoding

* 1.7x faster when encoding at quality level of 90%

Before you import library, you need to install libjpeg-turbo.

On Ubuntu: `sudo apt-get install libjpeg-turbo8-dev`.

On Mac OS X: `brew install libjpeg-turbo`
*/
package golibjpegturbo

// Note: on mac (darwin) /usr/local/opt symlinks to the latest installed version
// e.g. /usr/local/Cellar/jpeg-turbo/1.3.1

/*
#cgo linux LDFLAGS: -ljpeg
#cgo darwin LDFLAGS: -L/usr/local/opt/jpeg-turbo/lib -ljpeg
#cgo darwin CFLAGS: -I/usr/local/opt/jpeg-turbo/include

#include <stddef.h>
#include <stdio.h>
#include <stdlib.h>
#include <jpeglib.h>

void error_panic(j_common_ptr cinfo);
*/
import "C"

import (
	"fmt"
	"image"
	"io"
	"io/ioutil"
	"reflect"
	"unsafe"
)

// JpegInfo contains information about JPEG image.
type JpegInfo struct {
	Components       int
	ColorSpace       int
	Width            int
	Height           int
	ColorSpaceString string
}

/*
valid combinations of number of components vs. color space:

JCS_GRAYSCALE => 1
JCS_RGB, case JCS_YCbCr => 3
JCS_CMYK, JCS_YCCK => 4
*/

func colorSpaceToString(cs int) string {
	if cs == int(C.JCS_GRAYSCALE) {
		return "grayscale"
	}
	if cs == int(C.JCS_YCbCr) {
		return "ycbcr"
	}
	if cs == int(C.JCS_RGB) {
		return "rgb"
	}
	if cs == int(C.JCS_CMYK) {
		return "cmyk"
	}
	if cs == int(C.JCS_YCCK) {
		return "ycck"
	}
	// those seem only to be available for out_color_space i.e. decoded space
	if cs == int(C.JCS_EXT_RGB) {
		return "extrgb"
	}
	if cs == int(C.JCS_EXT_RGBX) {
		return "extrgbx"
	}
	if cs == int(C.JCS_EXT_BGR) {
		return "extbgr"
	}
	if cs == int(C.JCS_EXT_BGRX) {
		return "extbgrx"
	}
	if cs == int(C.JCS_EXT_XBGR) {
		return "extxbgr"
	}
	if cs == int(C.JCS_EXT_XRGB) {
		return "extxrgb"
	}
	if cs == int(C.JCS_EXT_RGBA) {
		return "extrgba"
	}
	if cs == int(C.JCS_EXT_BGRA) {
		return "extbgra"
	}
	if cs == int(C.JCS_EXT_ABGR) {
		return "extabgr"
	}
	if cs == int(C.JCS_EXT_ARGB) {
		return "extargb"
	}
	return "unknown"
}

// GetJpegInfo returns information about a JPEG image.
func GetJpegInfo(d []byte) (info *JpegInfo, err error) {
	defer func() {
		if r := recover(); r != nil {
			info = nil
			var ok bool
			err, ok = r.(error)
			if !ok {
				err = fmt.Errorf("JPEG error: %v", r)
			}
		}
	}()
	// those are allocated, not on stack because of
	// https://groups.google.com/forum/#!topic/golang-nuts/g4yBziN-MZQ
	cinfo := (*C.struct_jpeg_decompress_struct)(C.malloc(C.size_t(unsafe.Sizeof(C.struct_jpeg_decompress_struct{}))))
	defer C.free(unsafe.Pointer(cinfo))
	cinfo.err = (*C.struct_jpeg_error_mgr)(C.malloc(C.size_t(unsafe.Sizeof(C.struct_jpeg_error_mgr{}))))
	defer C.free(unsafe.Pointer(cinfo.err))

	C.jpeg_std_error(cinfo.err)
	cinfo.err.error_exit = (*[0]byte)(C.error_panic)

	C.jpeg_CreateDecompress(cinfo, C.JPEG_LIB_VERSION, C.size_t(unsafe.Sizeof(C.struct_jpeg_decompress_struct{})))
	defer C.jpeg_destroy_decompress(cinfo)

	// TODO: should make a copy in C memory for GC safety?
	C.jpeg_mem_src(cinfo, (*C.uchar)(unsafe.Pointer(&d[0])), C.ulong(len(d)))

	res := C.jpeg_read_header(cinfo, C.TRUE)
	if res != C.JPEG_HEADER_OK {
		err = fmt.Errorf("C.jpeg_reader_header() failed with %d", int(res))
		return
	}
	info = &JpegInfo{}
	info.Components = int(cinfo.num_components)
	info.ColorSpace = int(cinfo.jpeg_color_space)
	info.Width = int(cinfo.output_width)
	info.Height = int(cinfo.output_height)
	info.ColorSpaceString = colorSpaceToString(info.ColorSpace)
	return
}

func decodeToGray(cinfo *C.struct_jpeg_decompress_struct) image.Image {
	dx := int(cinfo.output_width)
	nBytes := dx // per one line, 1 byte per pixel
	dy := int(cinfo.output_height)
	img := image.NewGray(image.Rect(0, 0, dx, dy))

	// Note: for even greater speed we could decode directly into img.Pix
	// but that might stop working when moving GC happens
	bufBytes := C.malloc(C.size_t(nBytes))
	scanlines := C.JSAMPARRAY(unsafe.Pointer(&bufBytes))
	buf := sliceFromCBytes(bufBytes, nBytes)

	for y := 0; y < dy; y++ {
		C.jpeg_read_scanlines(cinfo, scanlines, 1)
		off := y * img.Stride
		copy(img.Pix[off:off+nBytes], buf)
	}
	C.free(bufBytes)
	return img
}

// based on https://code.google.com/p/go-wiki/wiki/cgo
// The important thing is that it creates []byte slice without copying
// memory, for speed
func sliceFromCBytes(p unsafe.Pointer, size int) []byte {
	hdr := reflect.SliceHeader{
		Data: uintptr(p),
		Len:  size,
		Cap:  size,
	}
	slice := *(*[]byte)(unsafe.Pointer(&hdr))
	return slice
}

// Note: for YCbCr could try to stay within YCbCr space, which might make the
// decode->resize->encode loop faster if we avoid conversion to RGBA at any point
// However, decoding to YCbCr is more complicated, because it has multiple
// variants (4:2:2 etc.)
func decodeToRgba(cinfo *C.struct_jpeg_decompress_struct) image.Image {
	dx := int(cinfo.output_width)
	nBytes := dx * 4 // per one line, 4 bytes of destination rgba per pixel
	dy := int(cinfo.output_height)
	img := image.NewRGBA(image.Rect(0, 0, dx, dy))

	// Note: for even greater speed we could decode directly into img.Pix
	// but that might stop working when moving GC happens
	bufBytes := C.malloc(C.size_t(nBytes))
	scanlines := C.JSAMPARRAY(unsafe.Pointer(&bufBytes))
	buf := sliceFromCBytes(bufBytes, nBytes)

	for y := 0; y < dy; y++ {
		C.jpeg_read_scanlines(cinfo, scanlines, 1)
		off := y * img.Stride
		copy(img.Pix[off:off+nBytes], buf)
	}

	C.free(bufBytes)
	return img
}

// Source is 'Inverted CMYK'
// See https://github.com/google/skia/blob/master/src/images/SkImageDecoder_libjpeg.cpp#L340
// for explanation
func decodeCmykToRgba(cinfo *C.struct_jpeg_decompress_struct) image.Image {
	dx := int(cinfo.output_width)
	nBytes := dx * 4 // per one line, 4 bytes of destination rgba per pixel
	dy := int(cinfo.output_height)
	img := image.NewRGBA(image.Rect(0, 0, dx, dy))

	bufBytes := C.malloc(C.size_t(nBytes))
	scanlines := C.JSAMPARRAY(unsafe.Pointer(&bufBytes))
	buf := sliceFromCBytes(bufBytes, nBytes)

	for y := 0; y < dy; y++ {
		C.jpeg_read_scanlines(cinfo, scanlines, 1)
		off := y * img.Stride
		srcOff := 0
		for x := 0; x < dx; x++ {
			c := uint32(buf[srcOff])
			srcOff++
			m := uint32(buf[srcOff])
			srcOff++
			y := uint32(buf[srcOff])
			srcOff++
			k := uint32(buf[srcOff])
			srcOff++

			r := uint8(c * k / 255)
			g := uint8(m * k / 255)
			b := uint8(y * k / 255)

			img.Pix[off] = r
			off++
			img.Pix[off] = g
			off++
			img.Pix[off] = b
			off++
			img.Pix[off] = 255
			off++
		}
	}
	C.free(bufBytes)
	return img
}

// DecodeData reads JPEG image from d and returns it as an image.Image.
func DecodeData(d []byte) (img image.Image, err error) {
	defer func() {
		if r := recover(); r != nil {
			img = nil
			var ok bool
			err, ok = r.(error)
			if !ok {
				err = fmt.Errorf("JPEG error: %v", r)
			}
		}
	}()

	// those are allocated from heap, not on stack because of
	// https://groups.google.com/forum/#!topic/golang-nuts/g4yBziN-MZQ
	cinfo := (*C.struct_jpeg_decompress_struct)(C.malloc(C.size_t(unsafe.Sizeof(C.struct_jpeg_decompress_struct{}))))
	defer C.free(unsafe.Pointer(cinfo))
	cinfo.err = (*C.struct_jpeg_error_mgr)(C.malloc(C.size_t(unsafe.Sizeof(C.struct_jpeg_error_mgr{}))))
	defer C.free(unsafe.Pointer(cinfo.err))

	C.jpeg_std_error(cinfo.err)
	cinfo.err.error_exit = (*[0]byte)(C.error_panic)

	C.jpeg_CreateDecompress(cinfo, C.JPEG_LIB_VERSION, C.size_t(unsafe.Sizeof(C.struct_jpeg_decompress_struct{})))
	defer C.jpeg_destroy_decompress(cinfo)

	// TODO: should make a copy in C memory for GC safety?
	C.jpeg_mem_src(cinfo, (*C.uchar)(unsafe.Pointer(&d[0])), C.ulong(len(d)))

	res := C.jpeg_read_header(cinfo, C.TRUE)
	if res != C.JPEG_HEADER_OK {
		img = nil
		err = fmt.Errorf("C.jpeg_reader_header() failed with %d", int(res))
		return
	}
	nComp := int(cinfo.num_components)

	// if we're decoding YCbCr image, ask libjpeg to decode directly to RGBA
	// for speed (as opposed to converting to RGB and doing RGB -> RGBA in Go)
	// Note: I don't know if JCS_EXT_RGBA is pre-multiplied alpha (like Go)
	// but it shouldn't matter for decoding (jpeg doesn't have alpha
	// information, so alpha component should always end up 0xff)
	if nComp == 3 {
		cinfo.out_color_space = C.JCS_EXT_RGBA
	}

	C.jpeg_start_decompress(cinfo)
	defer C.jpeg_finish_decompress(cinfo)

	if nComp == 1 {
		img = decodeToGray(cinfo)
	} else if nComp == 3 {
		img = decodeToRgba(cinfo)
	} else if nComp == 4 {
		img = decodeCmykToRgba(cinfo)
	} else {
		err = fmt.Errorf("Invalid number of components (%d)", cinfo.num_components)
	}
	return
}

// Decode reads a JPEG image from r and returns it as an image.Image.
func Decode(r io.Reader) (image.Image, error) {
	// loading the whole image is not ideal but doing callbacks is more complicated
	// so for now do the simple thing
	d, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return DecodeData(d)
}
