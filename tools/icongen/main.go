// Command icongen writes the launcher's .ico from the same pixels internal/art
// renders for the runtime icon, so the executable, the tray and the settings row
// cannot show different marks.
//
//	go generate ./tools/icongen
//
// The result is embedded into the binary by rsrc (see assets/README.md).
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"applauncher/internal/art"
)

//go:generate go run . -out ../../assets/launcher.ico

// Sizes Windows actually asks for. 256 is stored as PNG because a raw bitmap at
// that size would add a quarter of a megabyte to the binary.
var sizes = []int{16, 20, 24, 32, 40, 48, 64, 256}

const pngSize = 256

func main() {
	out := flag.String("out", "launcher.ico", "path of the .ico to write")
	flag.Parse()

	data, err := buildIcon()
	if err != nil {
		fmt.Fprintln(os.Stderr, "icongen:", err)
		os.Exit(1)
	}
	if dir := filepath.Dir(*out); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "icongen:", err)
			os.Exit(1)
		}
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "icongen:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d bytes, %d images)\n", *out, len(data), len(sizes))
}

func buildIcon() ([]byte, error) {
	images := make([][]byte, len(sizes))
	for i, size := range sizes {
		pixels := art.RenderBGRA(int32(size))
		if size == pngSize {
			encoded, err := encodePNG(pixels, size)
			if err != nil {
				return nil, err
			}
			images[i] = encoded
			continue
		}
		images[i] = encodeDIB(pixels, size)
	}

	// ICONDIR is 6 bytes, then 16 bytes per image, then the image data.
	offset := 6 + 16*len(images)
	buf := new(bytes.Buffer)
	write := func(v any) {
		if err := binary.Write(buf, binary.LittleEndian, v); err != nil {
			panic(err) // writing to a bytes.Buffer cannot fail
		}
	}

	write(uint16(0)) // reserved
	write(uint16(1)) // type: icon
	write(uint16(len(images)))

	for i, image := range images {
		size := sizes[i]
		dim := byte(size)
		if size >= 256 {
			dim = 0 // 0 means 256 in an .ico directory entry
		}
		write(dim)                // width
		write(dim)                // height
		write(byte(0))            // palette entries
		write(byte(0))            // reserved
		write(uint16(1))          // planes
		write(uint16(32))         // bits per pixel
		write(uint32(len(image))) // size of the image data
		write(uint32(offset))     // offset of the image data
		offset += len(image)
	}

	for _, image := range images {
		buf.Write(image)
	}
	return buf.Bytes(), nil
}

// encodeDIB packs the pixels as a bottom-up 32bpp bitmap plus the 1bpp AND mask
// that every .ico entry is required to carry.
func encodeDIB(pixels []byte, size int) []byte {
	const headerSize = 40
	maskStride := ((size + 31) / 32) * 4 // 1bpp rows are DWORD aligned

	buf := new(bytes.Buffer)
	write := func(v any) { _ = binary.Write(buf, binary.LittleEndian, v) }

	// BITMAPINFOHEADER. A positive height means the XOR and AND masks follow,
	// which is why it is doubled.
	write(uint32(headerSize))
	write(int32(size))
	write(int32(size * 2))
	write(uint16(1))  // planes
	write(uint16(32)) // bit count
	write(uint32(0))  // BI_RGB
	write(uint32(size * size * 4))
	write(int32(0))  // x pixels per meter
	write(int32(0))  // y pixels per meter
	write(uint32(0)) // colours used
	write(uint32(0)) // important colours

	// Rows are bottom-up.
	for y := size - 1; y >= 0; y-- {
		buf.Write(pixels[y*size*4 : (y+1)*size*4])
	}

	// AND mask: a set bit means transparent; Windows uses it on legacy paths,
	// the alpha channel on modern ones.
	mask := make([]byte, maskStride*size)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if pixels[(y*size+x)*4+3] == 0 {
				mask[y*maskStride+x/8] |= 0x80 >> (x % 8)
			}
		}
	}
	buf.Write(mask)

	return buf.Bytes()
}

// encodePNG stores the pixels as a straight RGBA PNG, which is what Windows
// expects for the 256x256 entry.
func encodePNG(pixels []byte, size int) ([]byte, error) {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for i := 0; i < size*size; i++ {
		b, g, r, a := pixels[i*4], pixels[i*4+1], pixels[i*4+2], pixels[i*4+3]
		img.Pix[i*4+0] = r
		img.Pix[i*4+1] = g
		img.Pix[i*4+2] = b
		img.Pix[i*4+3] = a
	}

	buf := new(bytes.Buffer)
	if err := png.Encode(buf, img); err != nil {
		return nil, fmt.Errorf("encode %dx%d png: %w", size, size, err)
	}
	return buf.Bytes(), nil
}
