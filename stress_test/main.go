package main

// stress test to validate encoding/decoding doesn't crash or have inconsistent
// results due to gc interaction with cgo code. To run:
// go run stress_test/main.go -dir=<directory with JPEG images>
import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/kjk/golibjpegturbo"
	"github.com/kr/fs"
)

func panicIfErr(err error) {
	if err != nil {
		panic(err.Error())
	}
}

var (
	nEncoded          int32
	nTotalEncodedSize int64
	mu                sync.Mutex
)

type ImageInfo struct {
	path        string
	data        []byte
	img         image.Image
	encodedData []byte
}

func encodeLibjpeg(img image.Image) []byte {
	var buf bytes.Buffer
	options := &golibjpegturbo.Options{Quality: 90}
	err := golibjpegturbo.Encode(&buf, img, options)
	panicIfErr(err)
	return buf.Bytes()
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func userHomeDir() string {
	// user.Current() returns nil if cross-compiled e.g. on mac for linux
	if usr, _ := user.Current(); usr != nil {
		return usr.HomeDir
	}
	return os.Getenv("HOME")
}

func expandTildeInPath(s string) string {
	if strings.HasPrefix(s, "~") {
		return userHomeDir() + s[1:]
	}
	return s
}

func isJpegFile(path string) bool {
	ext := filepath.Ext(path)
	ext = strings.ToLower(ext)
	return ext == ".jpg" || ext == ".jpeg"
}

func validateImgEq(img1, img2 image.Image) {
	same := reflect.DeepEqual(img1, img2)
	if !same {
		panic("decoded image not consistent across runs")
	}
}

func decodeEncodeWorker(c chan *ImageInfo) {
	for ii := range c {
		d := ii.data
		r := bytes.NewReader(d)
		img, err := golibjpegturbo.Decode(r)
		if err != nil {
			// we have decoded the image during setup, so this should always succeed
			panic(fmt.Sprintf("failed to decode %s with %s\n", ii.path, err))
		}
		validateImgEq(ii.img, img)
		encoded := encodeLibjpeg(img)
		mu.Lock()
		if ii.encodedData == nil {
			ii.encodedData = encoded
		}
		mu.Unlock()
		if !bytes.Equal(encoded, ii.encodedData) {
			panic("encoded data not consistent across runs")
		}
		atomic.AddInt64(&nTotalEncodedSize, int64(len(encoded)))
		n := atomic.AddInt32(&nEncoded, 1)
		if n%100 == 0 {
			fmt.Printf("Decoded/encoded %d images\n", n)
		}
	}
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	var flagDir string
	flag.StringVar(&flagDir, "dir", "", "directory with images")
	flag.Parse()

	if flagDir == "" {
		flag.Usage()
		os.Exit(2)
	}
	dir := expandTildeInPath(flagDir)
	if !pathExists(dir) {
		fmt.Printf("dir %s doesn't exist\n", dir)
		flag.Usage()
		os.Exit(2)
	}
	walker := fs.Walk(dir)
	var imagePaths []string
	nMaxImages := 100
	for walker.Step() {
		st := walker.Stat()
		if !st.Mode().IsRegular() {
			continue
		}
		path := walker.Path()
		if !isJpegFile(path) {
			continue
		}
		imagePaths = append(imagePaths, path)
		if len(imagePaths) >= nMaxImages {
			break
		}
	}
	if len(imagePaths) == 0 {
		fmt.Printf("There are no jpeg images in %s\n", dir)
		flag.Usage()
		os.Exit(2)
	}
	var images []*ImageInfo
	for _, path := range imagePaths {
		data, err := ioutil.ReadFile(path)
		if err != nil {
			fmt.Printf("ioutil.ReadFile() failed with %s\n", err)
			continue
		}
		img, err := golibjpegturbo.DecodeData(data)
		if err != nil {
			fmt.Printf("Failed to decode %s with %s\n", path, err)
			continue
		}
		ii := &ImageInfo{
			path: path,
			data: data,
			img:  img,
		}
		images = append(images, ii)
	}
	fmt.Printf("Read %d images\n", len(images))
	c := make(chan *ImageInfo)
	nWorkers := runtime.NumCPU() - 2 // don't fully overload the machine
	if nWorkers < 1 {
		nWorkers = 1
	}
	fmt.Printf("Staring %d workers\n", nWorkers)
	for i := 0; i < nWorkers; i++ {
		go decodeEncodeWorker(c)
	}
	fmt.Printf("To stop me, use Ctrl-C. Otherwise, I'll just keep going\n")
	i := 0
	for {
		c <- images[i]
		i++
		i = i % len(images)
	}
}
