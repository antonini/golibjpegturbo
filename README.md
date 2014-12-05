## golibjpegturbo

This library is the fastest possible way to decode and encode JPEG images in Go.

We achieve this by doing cgo binding to [libjpeg-turbo](libjpeg-turbo.virtualgl.org)
library.

The exact speed changes depend on the image and CPU. On Mac Book Pro, compared
to `image/jpeg` standard library, golibjpegturbo is:
* 6x faster when decoding
* 1.7x faster when encoding at same quality level of 90%

You can rerun the benchmark on your machine with `go test -bench=.`

Additionally, the library can read jpeg images in CMYK format.

## Setup

Before you import library, you need to install libjpeg-turbo.

On Ubuntu: `sudo apt-get install libjpeg-turbo8-dev`.

On Mac OS X: `apt-get install libjpeg-turbo`

## Usage

`go get github.com/kjk/golibjpegturbo`

The API is the same as `image/libjpeg`:

```go
import "libjpeg"

func decode(r io.Reader) (image.Image, error) {
    return libjpeg.Decode(r)
}

func encode(img image.Image) ([]byte, error) {
    options := &libjpeg.Options{Quality: 90}
    var buf bytes.Buffer
    err = Encode(&buf, decodedImg, options)
    if err != nil {
        return nil, err
    }
    return buf.Bytes(), nil
}

```

## License

MIT.
