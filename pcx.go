package pcx

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"fmt"
	"io"
	"io/ioutil"
)

// Header holds information about a pcx file
type Header struct {
	Marker               byte // The first byte of a PCX file is 0x0a.
	Version              byte // The second byte (the version number) is 0, 2, 3, 4, or 5.
	Encoding             byte // The third byte (the encoding) is 1 or (rarely) 0.
	BitsPerPixelPerPlane byte
	WindowXMin           uint16 // ImageWidth = Xmax - Xmin + 1
	WindowYMin           uint16 // ImageHeight = Ymax - Ymin + 1
	WindowXMax           uint16 // a 200px high image would have Ymin=0 Ymax=199
	WindowYMax           uint16
	VerticalDPI          uint16   // unreliable
	HorizontalDPI        uint16   // unreliable
	Palette16            [48]byte // 16 color palette in header
	_                    byte
	NumPlanes            byte
	BytesPerPlaneLine    uint16
	PaletteInfo          uint16 // How to interpret the palette 1 = Color/BW 2 = Grayscale
	HorizontalScreenSize uint16
	VerticalScreenSize   uint16
	_                    [54]byte // padding
}

// Container holds the raw PCX header and data.
type Container struct {
	Header  Header
	Data    []byte
	Palette []color.Color
}

// Decode8Bit256Color decodes 8-bit 256 color pcx data into an image.
func Decode8Bit256Color(r io.Reader) (img image.Image, err error) {
	var (
		raw      Container
		paletted *image.Paletted
		buf      []byte
	)
	buf, err = ioutil.ReadAll(r)
	if err != nil {
		return
	}
	pcxBytes := bytes.NewReader(buf)
	raw, err = loadImage(pcxBytes)
	if err != nil {
		return
	}
	if raw.Header.BitsPerPixelPerPlane != 8 {
		err = fmt.Errorf("pcx: header says %d bits per pixel, expecting 8", raw.Header.BitsPerPixelPerPlane)
		return
	}
	if raw.Header.NumPlanes != 1 {
		err = fmt.Errorf("pcx: header says %d planes, expecting 1", raw.Header.NumPlanes)
		return
	}
	paletted, err = raw.palettedFromContainer()
	if err != nil {
		return
	}
	img = paletted.SubImage(paletted.Rect)
	return
}

func (c *Container) palettedFromContainer() (*image.Paletted, error) {
	result := &image.Paletted{}
	newRect := image.Rectangle{
		Min: image.Point{int(c.Header.WindowXMin), int(c.Header.WindowYMin)},
		Max: image.Point{int(c.Header.WindowYMax) + 1, int(c.Header.WindowYMax) + 1},
	}
	decompressed, err := decompressWithRLE(c.Data)
	if err != nil {
		return nil, err
	}
	dataReader := bytes.NewReader(decompressed)
	result.Rect = newRect
	result.Stride = int(c.Header.BytesPerPlaneLine)
	result.Pix = make([]uint8, (result.Rect.Max.Y+1)*(result.Rect.Max.X+1))
	result.Palette = c.Palette
	for y := 0; y <= int(c.Header.WindowYMax); y++ {
		for x := 0; x <= int(c.Header.WindowXMax); x++ {
			index, err := dataReader.ReadByte()
			if err != nil {
				if err != io.EOF {
					return nil, err
				}
			}
			result.Set(x, y, c.Palette[int(index)])
		}
	}
	return result, nil
}
func decompressWithRLE(compressed []byte) ([]byte, error) {
	var (
		repeat    int
		repeatSet bool
		out       bytes.Buffer
		buf       [64]byte
	)
	in := bytes.NewReader(compressed)
	for i := 0; i < len(compressed); i++ {
		c, err := in.ReadByte()
		if err != nil {
			return nil, err
		}
		if !repeatSet {
			if c >= 192 {
				repeat = int(c & 0x3f)
				repeatSet = true
			} else {
				err = out.WriteByte(c)
				if err != nil {
					return nil, err
				}
			}
		} else {
			if repeat > 0 {
				for j := 0; j < repeat; j++ {
					buf[j] = c
				}
				if n, err := out.Write(buf[:repeat]); n != repeat || err != nil {
					return nil, io.ErrShortWrite
				}
			}
			repeat = 0
			repeatSet = false
		}
	}
	return out.Bytes(), nil
}
func loadImage(r io.ReadSeeker) (Container, error) {
	result := Container{}
	headerRaw := make([]byte, 128)
	_, err := r.Read(headerRaw)
	if err != nil {
		return result, err
	}
	err = binary.Read(bytes.NewReader(headerRaw), binary.LittleEndian, &result.Header)
	if err != nil {
		return result, err
	}
	n, err := r.Seek(-0x300, io.SeekEnd)
	if err != nil {
		return result, err
	}
	_, err = r.Seek(128, io.SeekStart)
	if err != nil {
		return result, err
	}
	result.Data = make([]byte, n-129)
	_, err = r.Read(result.Data)
	if err != nil {
		return result, err
	}
	_, err = r.Seek(1, io.SeekCurrent)
	if err != nil {
		return result, err
	}
	rawPalette := make([]byte, 0x300)
	_, err = r.Read(rawPalette)
	if err != nil {
		return result, err
	}
	for i := 0; i < 0x300; i += 3 {
		pc := color.RGBA{rawPalette[i], rawPalette[i+1], rawPalette[i+2], 0xff}
		result.Palette = append(result.Palette, pc)
	}
	return result, nil
}
