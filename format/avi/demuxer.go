package avi

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"time"

	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/codec"
	"github.com/deepch/vdk/codec/aacparser"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/codec/h265parser"
	"github.com/deepch/vdk/format/avi/aviio"
)

type Demuxer struct {
	r            *bufio.Reader
	streams      []av.CodecData
	mainHeader   *aviio.MainAVIHeader
	streamInfos  []streamInfo
	indexEntries []aviio.IndexEntry
	moviOffset   int64
	currentFrame int
}

type streamInfo struct {
	streamHeader *aviio.StreamHeader
	codecData    av.CodecData
	isVideo      bool
	isAudio      bool
	streamIndex  int
}

func NewDemuxer(r io.Reader) *Demuxer {
	return &Demuxer{
		r: bufio.NewReader(r),
	}
}

func (d *Demuxer) Streams() ([]av.CodecData, error) {
	if d.streams != nil {
		return d.streams, nil
	}

	// Read RIFF header
	riffHeader, err := aviio.ReadChunkHeader(d.r)
	if err != nil {
		return nil, err
	}
	if riffHeader.FourCC != aviio.FourCCRIFF {
		return nil, aviio.ErrInvalidFormat
	}

	// Read AVI signature
	var aviSig uint32
	if err := binary.Read(d.r, binary.LittleEndian, &aviSig); err != nil {
		return nil, err
	}
	if aviSig != aviio.FourCCAVI {
		return nil, aviio.ErrInvalidFormat
	}

	// Parse headers
	if err := d.parseHeaders(); err != nil {
		return nil, err
	}

	// Build streams array
	d.streams = make([]av.CodecData, 0, len(d.streamInfos))
	for _, info := range d.streamInfos {
		d.streams = append(d.streams, info.codecData)
	}

	return d.streams, nil
}

func (d *Demuxer) parseHeaders() error {
	for {
		header, err := aviio.ReadChunkHeader(d.r)
		if err != nil {
			return err
		}

		switch header.FourCC {
		case aviio.FourCCLIST:
			var listType uint32
			if err := binary.Read(d.r, binary.LittleEndian, &listType); err != nil {
				return err
			}

			switch listType {
			case aviio.FourCChdrl:
				if err := d.parseHdrlList(header.Size - 4); err != nil {
					return err
				}
			case aviio.FourCCmovi:
				// Record offset to movi chunk for later seeking
				d.moviOffset, _ = d.getPosition()
				d.moviOffset -= 4 // Account for LIST type we already read
				// Skip the movi chunk for now
				if _, err := d.r.Discard(int(header.Size - 4)); err != nil {
					return err
				}
			default:
				// Skip unknown LIST chunks
				if _, err := d.r.Discard(int(header.Size - 4)); err != nil {
					return err
				}
			}

		case aviio.FourCCidx1:
			// Parse index
			if err := d.parseIndex(header.Size); err != nil {
				return err
			}
			// We're done parsing headers after index
			return nil

		default:
			// Skip unknown chunks
			if _, err := d.r.Discard(int(header.Size)); err != nil {
				return err
			}
		}

		// Align to word boundary
		if header.Size&1 == 1 {
			d.r.ReadByte()
		}
	}
}

func (d *Demuxer) parseHdrlList(size uint32) error {
	endPos := size
	bytesRead := uint32(0)

	for bytesRead < endPos {
		header, err := aviio.ReadChunkHeader(d.r)
		if err != nil {
			return err
		}
		bytesRead += 8

		switch header.FourCC {
		case aviio.FourCCavih:
			d.mainHeader, err = aviio.ReadMainAVIHeader(d.r)
			if err != nil {
				return err
			}
		case aviio.FourCCLIST:
			var listType uint32
			if err := binary.Read(d.r, binary.LittleEndian, &listType); err != nil {
				return err
			}
			if listType == aviio.FourCCstrl {
				if err := d.parseStrlList(header.Size - 4); err != nil {
					return err
				}
			} else {
				// Skip unknown LIST
				if _, err := d.r.Discard(int(header.Size - 4)); err != nil {
					return err
				}
			}
		default:
			// Skip unknown chunks
			if _, err := d.r.Discard(int(header.Size)); err != nil {
				return err
			}
		}

		bytesRead += header.Size
		// Align to word boundary
		if header.Size&1 == 1 {
			d.r.ReadByte()
			bytesRead++
		}
	}

	return nil
}

func (d *Demuxer) parseStrlList(size uint32) error {
	var info streamInfo
	info.streamIndex = len(d.streamInfos)

	endPos := size
	bytesRead := uint32(0)

	for bytesRead < endPos {
		header, err := aviio.ReadChunkHeader(d.r)
		if err != nil {
			return err
		}
		bytesRead += 8

		switch header.FourCC {
		case aviio.FourCCstrh:
			info.streamHeader, err = aviio.ReadStreamHeader(d.r)
			if err != nil {
				return err
			}
			info.isVideo = info.streamHeader.Type == aviio.FourCCvids
			info.isAudio = info.streamHeader.Type == aviio.FourCCauds

		case aviio.FourCCstrf:
			data := make([]byte, header.Size)
			if _, err := io.ReadFull(d.r, data); err != nil {
				return err
			}

			if info.isVideo {
				// Parse video format
				r := bytes.NewReader(data)
				bih, err := aviio.ReadBitmapInfoHeader(r)
				if err != nil {
					return err
				}

				// Extract codec based on compression type
				switch aviio.FourCCString(bih.Compression) {
				case "H264", "h264", "avc1", "AVC1":
					// Extract SPS/PPS from extradata if available
					extraDataSize := int(header.Size) - 40 // BitmapInfoHeader is 40 bytes
					if extraDataSize > 0 {
						extraData := data[40:]
						codec, err := h264parser.NewCodecDataFromAVCDecoderConfRecord(extraData)
						if err == nil {
							info.codecData = codec
						} else {
							// Create basic H264 codec data
							info.codecData = &h264parser.CodecData{}
						}
					} else {
						info.codecData = &h264parser.CodecData{}
					}

				case "H265", "h265", "hvc1", "HVC1", "hevc", "HEVC":
					extraDataSize := int(header.Size) - 40
					if extraDataSize > 0 {
						extraData := data[40:]
						codec, err := h265parser.NewCodecDataFromAVCDecoderConfRecord(extraData)
						if err == nil {
							info.codecData = codec
						} else {
							info.codecData = &h265parser.CodecData{}
						}
					} else {
						info.codecData = &h265parser.CodecData{}
					}

				default:
					// Unsupported video codec
					info.codecData = nil
				}

			} else if info.isAudio {
				// Parse audio format
				r := bytes.NewReader(data)
				wfx, err := aviio.ReadWaveFormatEx(r)
				if err != nil {
					return err
				}

				// Extract codec based on format tag
				switch wfx.FormatTag {
				case 0xFF: // AAC
					if len(data) > 18 && wfx.CbSize > 0 {
						// Extract AAC specific config from extra data
						extraData := data[18:]
						codec, err := aacparser.NewCodecDataFromMPEG4AudioConfigBytes(extraData)
						if err == nil {
							info.codecData = codec
						} else {
							// Create basic AAC codec data
							info.codecData = &aacparser.CodecData{
								Config: aacparser.MPEG4AudioConfig{
									SampleRate:    int(wfx.SamplesPerSec),
									ChannelLayout: av.CH_STEREO,
									ObjectType:    aacparser.AOT_AAC_LC,
								},
							}
						}
					} else {
						info.codecData = &aacparser.CodecData{
							Config: aacparser.MPEG4AudioConfig{
								SampleRate:    int(wfx.SamplesPerSec),
								ChannelLayout: av.CH_STEREO,
								ObjectType:    aacparser.AOT_AAC_LC,
							},
						}
					}

				case 0x07: // PCM MULAW
					info.codecData = codec.NewPCMMulawCodecData()

				case 0x06: // PCM ALAW
					info.codecData = codec.NewPCMAlawCodecData()

				default:
					// Unsupported audio codec
					info.codecData = nil
				}
			}

		default:
			// Skip unknown chunks
			if _, err := d.r.Discard(int(header.Size)); err != nil {
				return err
			}
		}

		bytesRead += header.Size
		// Align to word boundary
		if header.Size&1 == 1 {
			d.r.ReadByte()
			bytesRead++
		}
	}

	if info.codecData != nil {
		d.streamInfos = append(d.streamInfos, info)
	}

	return nil
}

func (d *Demuxer) parseIndex(size uint32) error {
	numEntries := size / 16 // Each index entry is 16 bytes
	d.indexEntries = make([]aviio.IndexEntry, numEntries)

	for i := range d.indexEntries {
		err := binary.Read(d.r, binary.LittleEndian, &d.indexEntries[i])
		if err != nil {
			return err
		}
	}

	return nil
}

func (d *Demuxer) ReadPacket() (av.Packet, error) {
	if d.currentFrame >= len(d.indexEntries) {
		return av.Packet{}, io.EOF
	}

	entry := d.indexEntries[d.currentFrame]
	d.currentFrame++

	// Determine stream index from chunk ID
	// Format is typically "00dc" for video stream 0, "01wb" for audio stream 1, etc.
	chunkIDStr := aviio.FourCCString(entry.ChunkID)
	streamNum := int(chunkIDStr[0]-'0')*10 + int(chunkIDStr[1]-'0')

	var streamIdx int
	var isKeyFrame bool

	// Find the actual stream index
	for i, info := range d.streamInfos {
		if (info.isVideo && chunkIDStr[2:4] == "dc") ||
			(info.isVideo && chunkIDStr[2:4] == "db") ||
			(info.isAudio && chunkIDStr[2:4] == "wb") {
			if streamNum == i {
				streamIdx = i
				isKeyFrame = (entry.Flags & aviio.AVIIF_KEYFRAME) != 0
				break
			}
		}
	}

	// Read the chunk data
	data := make([]byte, entry.Size)
	// Note: In a real implementation, we'd need to seek to the correct position
	// For now, we'll assume sequential reading
	if _, err := io.ReadFull(d.r, data); err != nil {
		return av.Packet{}, err
	}

	// Calculate timestamp
	info := d.streamInfos[streamIdx]
	var ts time.Duration
	if info.isVideo {
		// For video, use frame number and fps
		if info.streamHeader.Rate > 0 && info.streamHeader.Scale > 0 {
			fps := float64(info.streamHeader.Rate) / float64(info.streamHeader.Scale)
			ts = time.Duration(float64(d.currentFrame-1) * float64(time.Second) / fps)
		}
	} else if info.isAudio {
		// For audio, use sample count
		if info.streamHeader.Rate > 0 {
			ts = time.Duration(d.currentFrame-1) * time.Second / time.Duration(info.streamHeader.Rate)
		}
	}

	return av.Packet{
		Idx:        int8(streamIdx),
		IsKeyFrame: isKeyFrame,
		Time:       ts,
		Data:       data,
	}, nil
}

func (d *Demuxer) getPosition() (int64, error) {
	// This is a simplified position tracking
	// In a real implementation, we'd need to track the actual file position
	return 0, nil
}
