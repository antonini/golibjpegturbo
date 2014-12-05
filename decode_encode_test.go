package golibjpegturbo

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"testing"
)

const (
	// a random 683x1024 441kb image
	imageUrl      = "https://farm1.staticflickr.com/47/139138903_3d9600174d_b_d.jpg"
	localFileName = "test.jpg"
	saveResized   = false
)

var (
	imgData    []byte
	imgGray    []byte
	imgCmyk    []byte
	decodedImg image.Image
	encodedImg []byte
)

func init() {
	// download test file if doesn't exist
	if !PathExists(localFileName) {
		d := httpDl(imageUrl)
		err := ioutil.WriteFile(localFileName, d, 0644)
		panicIfErr(err)
	}
	d, err := ioutil.ReadFile(localFileName)
	if err != nil {
		log.Fatalf("ReadFile() failed with %q\n", err)
	}
	imgData = d
	r := bytes.NewReader(d)
	decodedImg, err = Decode(r)
	if err != nil {
		log.Fatalf("Decode() failed with %q\n", err)
	}
	if !convertExists() {
		return
	}
	imgGray = convertAndLoad("gray")
	imgCmyk = convertAndLoad("cmyk")
}

func httpDl(uri string) []byte {
	res, err := http.Get(uri)
	panicIfErr(err)
	d, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	panicIfErr(err)
	return d
}

// treats any error (e.g. lack of access due to permissions) as non-existence
func PathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func panicIfErr(err error) {
	if err != nil {
		panic(err.Error())
	}
}

// see if ImageMagick's convert utility exists
func convertExists() bool {
	cmd := exec.Command("convert", "-version")
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

// convert test.jpeg to a given colorspace and load the data using ImageMagick's
// convert utility
func convertAndLoad(colorSpace string) []byte {
	tmpPath := "tmp-" + colorSpace + ".jpeg"
	cmd := exec.Command("convert", "test.jpg", "-colorspace", colorSpace, tmpPath)
	err := cmd.Run()
	panicIfErr(err)
	d, err := ioutil.ReadFile(tmpPath)
	panicIfErr(err)
	err = os.Remove(tmpPath)
	panicIfErr(err)
	return d
}

func ImageToRgba(img image.Image) *image.RGBA {
	switch v := img.(type) {
	case *image.RGBA:
		return v
	}
	// all other images we convert to RGBA because rez only supports YCbCr and RGBA
	// and there are weird restrictions on YCbCr width/height that are not met by
	// many YCbCr images
	r := img.Bounds()
	r = image.Rect(0, 0, r.Dx(), r.Dy())
	newImg := image.NewRGBA(r)
	draw.Draw(newImg, r, img, image.Point{}, draw.Src)
	return newImg
}

// not every image type has SubImage
func SubImage(img image.Image, r image.Rectangle) image.Image {
	// fast path for types we expect to encounter in real life
	switch v := img.(type) {
	case *image.RGBA:
		return v.SubImage(r)
	case *image.Gray:
		return v.SubImage(r)
	case *image.Paletted:
		return v.SubImage(r)
	case *image.NRGBA:
		return v.SubImage(r)
	case *image.Gray16:
		return v.SubImage(r)
	case *image.YCbCr:
		return v.SubImage(r)
	}
	// slow path for everything else
	img2 := ImageToRgba(img)
	return img2.SubImage(r)
}

func saveEncoded(t *testing.T, img image.Image, path string) {
	if !saveResized {
		return
	}
	fout, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer fout.Close()

	if err := Encode(fout, img, nil); err != nil {
		t.Fatal(err)
	}
}

func reencodeData(t *testing.T, imgData []byte, cs string) {
	var r image.Rectangle
	var img2 image.Image
	fin := bytes.NewBuffer(imgData)
	img, err := Decode(fin)
	if err != nil {
		t.Fatal(err)
	}
	saveEncoded(t, img, fmt.Sprintf("test_reencoded%s.jpg", cs))

	// test bounds (0, 0, dx/2, dy/2)
	r = img.Bounds()
	r.Max.X = r.Max.X / 2
	r.Max.Y = r.Max.Y / 2
	img2 = SubImage(img, r)
	saveEncoded(t, img2, fmt.Sprintf("test_reencoded%s_0.jpg", cs))

	// test bounds (dx/2, 0, dx, dy/2)
	r = img.Bounds()
	r.Min.X = r.Max.X / 2
	r.Max.Y = r.Max.Y / 2
	img2 = SubImage(img, r)
	saveEncoded(t, img2, fmt.Sprintf("test_reencoded%s_1.jpg", cs))

	// test bounds (0, dy/2, dx/2, dy)
	r = img.Bounds()
	r.Max.X = r.Max.X / 2
	r.Min.Y = r.Max.Y / 2
	img2 = SubImage(img, r)
	saveEncoded(t, img2, fmt.Sprintf("test_reencoded%s_2.jpg", cs))

	// test bounds (dx/2, dy/2, dx, dy)
	r = img.Bounds()
	r.Min.X = r.Max.X / 2
	r.Min.Y = r.Max.Y / 2
	img2 = SubImage(img, r)
	saveEncoded(t, img2, fmt.Sprintf("test_reencoded%s_3.jpg", cs))

	// test taking half from center
	r = img.Bounds()
	dx := r.Dx()
	dy := r.Dy()
	r.Min.X = dx / 4
	r.Max.X = r.Min.X + dx/2
	r.Min.Y = dy / 4
	r.Max.Y = r.Min.Y + dy/2
	img2 = SubImage(img, r)
	saveEncoded(t, img2, fmt.Sprintf("test_reencoded%s_4.jpg", cs))

	// test bounds taking 1 px from each side
	r = img.Bounds()
	r.Min.X = 1
	r.Min.Y = 1
	r.Max.X--
	r.Max.Y--
	img2 = SubImage(img, r)
	saveEncoded(t, img2, fmt.Sprintf("test_reencoded%s_5.jpg", cs))
}

func TestReencode(t *testing.T) {
	reencodeData(t, imgData, "")
	if len(imgGray) > 0 {
		reencodeData(t, imgGray, "_gray")
	}
	if len(imgCmyk) > 0 {
		reencodeData(t, imgGray, "_cmyk")
	}
}

func BenchmarkDecode(b *testing.B) {
	var err error
	for n := 0; n < b.N; n++ {
		r := bytes.NewBuffer(imgData)
		decodedImg, err = Decode(r)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeGo(b *testing.B) {
	var err error
	for n := 0; n < b.N; n++ {
		r := bytes.NewBuffer(imgData)
		decodedImg, err = jpeg.Decode(r)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncode(b *testing.B) {
	var err error
	options := &Options{Quality: 90}
	var buf bytes.Buffer
	for n := 0; n < b.N; n++ {
		buf.Reset()
		err = Encode(&buf, decodedImg, options)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeGo(b *testing.B) {
	var err error
	options := &jpeg.Options{Quality: 90}
	var buf bytes.Buffer
	for n := 0; n < b.N; n++ {
		buf.Reset()
		err = jpeg.Encode(&buf, decodedImg, options)
		if err != nil {
			b.Fatal(err)
		}
	}
}
