package main

// trying to repro libjpeg.Encode() crash with 1.4
// To run: go run repro_crash/main.go
import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
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
)

func encodeLibjpeg(img image.Image) int {
	var buf bytes.Buffer
	options := &golibjpegturbo.Options{Quality: 90}
	err := golibjpegturbo.Encode(&buf, img, options)
	panicIfErr(err)
	return buf.Len()
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

func decodeEncodeWorker(c chan []byte) {
	for d := range c {
		r := bytes.NewReader(d)
		img, err := golibjpegturbo.Decode(r)
		if err != nil {
			continue
		}
		dataLen := encodeLibjpeg(img)
		atomic.AddInt64(&nTotalEncodedSize, int64(dataLen))
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
	var images [][]byte
	for _, path := range imagePaths {
		d, err := ioutil.ReadFile(path)
		if err != nil {
			fmt.Printf("ioutil.ReadFile() failed with %s\n", err)
			continue
		}
		images = append(images, d)
	}
	fmt.Printf("Read %d images\n", len(images))
	c := make(chan []byte)
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
