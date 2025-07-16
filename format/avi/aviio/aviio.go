package aviio

import (
	"encoding/binary"
	"errors"
	"io"
)

var (
	ErrInvalidFormat = errors.New("aviio: invalid AVI format")
	ErrUnexpectedEOF = errors.New("aviio: unexpected EOF")
)

// FourCC converts a 4-character string to uint32
func FourCC(s string) uint32 {
	if len(s) != 4 {
		panic("FourCC: string must be 4 characters")
	}
	return binary.LittleEndian.Uint32([]byte(s))
}

// FourCCString converts uint32 to 4-character string
func FourCCString(n uint32) string {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], n)
	return string(b[:])
}

// Common FourCC codes
var (
	FourCCRIFF = FourCC("RIFF")
	FourCCAVI  = FourCC("AVI ")
	FourCCLIST = FourCC("LIST")
	FourCChdrl = FourCC("hdrl")
	FourCCavih = FourCC("avih")
	FourCCstrl = FourCC("strl")
	FourCCstrh = FourCC("strh")
	FourCCstrf = FourCC("strf")
	FourCCmovi = FourCC("movi")
	FourCCidx1 = FourCC("idx1")
	FourCCvids = FourCC("vids")
	FourCCauds = FourCC("auds")
	FourCC00dc = FourCC("00dc") // Compressed video
	FourCC00db = FourCC("00db") // Uncompressed video
	FourCC01wb = FourCC("01wb") // Audio data
)

// ChunkHeader represents a RIFF chunk header
type ChunkHeader struct {
	FourCC uint32
	Size   uint32
}

// MainAVIHeader represents the main AVI header structure
type MainAVIHeader struct {
	MicroSecPerFrame    uint32
	MaxBytesPerSec      uint32
	PaddingGranularity  uint32
	Flags               uint32
	TotalFrames         uint32
	InitialFrames       uint32
	Streams             uint32
	SuggestedBufferSize uint32
	Width               uint32
	Height              uint32
	Reserved            [4]uint32
}

// StreamHeader represents a stream header structure
type StreamHeader struct {
	Type                uint32 // 'vids' or 'auds'
	Handler             uint32 // codec FourCC
	Flags               uint32
	Priority            uint16
	Language            uint16
	InitialFrames       uint32
	Scale               uint32 // time scale
	Rate                uint32 // frames/samples per second * Scale
	Start               uint32
	Length              uint32 // stream length in Scale units
	SuggestedBufferSize uint32
	Quality             uint32
	SampleSize          uint32
	Frame               [4]uint16 // rcFrame
}

// BitmapInfoHeader for video streams
type BitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

// WaveFormatEx for audio streams
type WaveFormatEx struct {
	FormatTag      uint16
	Channels       uint16
	SamplesPerSec  uint32
	AvgBytesPerSec uint32
	BlockAlign     uint16
	BitsPerSample  uint16
	CbSize         uint16
}

// IndexEntry represents an entry in the idx1 chunk
type IndexEntry struct {
	ChunkID uint32
	Flags   uint32
	Offset  uint32 // offset from movi list
	Size    uint32
}

// Index flags
const (
	AVIIF_KEYFRAME = 0x00000010
)

// ReadChunkHeader reads a chunk header from reader
func ReadChunkHeader(r io.Reader) (*ChunkHeader, error) {
	var h ChunkHeader
	err := binary.Read(r, binary.LittleEndian, &h)
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// WriteChunkHeader writes a chunk header to writer
func WriteChunkHeader(w io.Writer, fourCC uint32, size uint32) error {
	h := ChunkHeader{FourCC: fourCC, Size: size}
	return binary.Write(w, binary.LittleEndian, &h)
}

// ReadMainAVIHeader reads the main AVI header
func ReadMainAVIHeader(r io.Reader) (*MainAVIHeader, error) {
	var h MainAVIHeader
	err := binary.Read(r, binary.LittleEndian, &h)
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// WriteMainAVIHeader writes the main AVI header
func WriteMainAVIHeader(w io.Writer, h *MainAVIHeader) error {
	return binary.Write(w, binary.LittleEndian, h)
}

// ReadStreamHeader reads a stream header
func ReadStreamHeader(r io.Reader) (*StreamHeader, error) {
	var h StreamHeader
	err := binary.Read(r, binary.LittleEndian, &h)
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// WriteStreamHeader writes a stream header
func WriteStreamHeader(w io.Writer, h *StreamHeader) error {
	return binary.Write(w, binary.LittleEndian, h)
}

// ReadBitmapInfoHeader reads a bitmap info header
func ReadBitmapInfoHeader(r io.Reader) (*BitmapInfoHeader, error) {
	var h BitmapInfoHeader
	err := binary.Read(r, binary.LittleEndian, &h)
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// WriteBitmapInfoHeader writes a bitmap info header
func WriteBitmapInfoHeader(w io.Writer, h *BitmapInfoHeader) error {
	return binary.Write(w, binary.LittleEndian, h)
}

// ReadWaveFormatEx reads a wave format header
func ReadWaveFormatEx(r io.Reader) (*WaveFormatEx, error) {
	var h WaveFormatEx
	err := binary.Read(r, binary.LittleEndian, &h)
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// WriteWaveFormatEx writes a wave format header
func WriteWaveFormatEx(w io.Writer, h *WaveFormatEx) error {
	return binary.Write(w, binary.LittleEndian, h)
}

// Align aligns n to 2-byte boundary
func Align(n int) int {
	return (n + 1) &^ 1
}