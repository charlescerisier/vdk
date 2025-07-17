package avi

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/codec/aacparser"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/codec/h265parser"
	"github.com/deepch/vdk/format/avi/aviio"
)

type Muxer struct {
	ws              io.WriteSeeker
	codecData       []av.CodecData
	videoStreamIdx  int
	audioStreamIdx  int
	hasVideo        bool
	hasAudio        bool
	frameCount      uint32
	indexEntries    []aviio.IndexEntry
	moviListPos     int64
	headerPos       int64
	dataSize        uint32
	fps             float64
	width           uint32
	height          uint32
	audioSampleRate uint32
}

func NewMuxer(ws io.WriteSeeker) *Muxer {
	return &Muxer{
		ws:             ws,
		videoStreamIdx: -1,
		audioStreamIdx: -1,
	}
}

func (m *Muxer) WriteHeader(codecData []av.CodecData) error {
	m.codecData = codecData

	// Identify stream types
	for i, cd := range codecData {
		if cd.Type().IsVideo() {
			m.videoStreamIdx = i
			m.hasVideo = true
			// Extract video parameters
			switch cd.Type() {
			case av.H264:
				if h264CD, ok := cd.(h264parser.CodecData); ok {
					sps := h264CD.SPS()
				if len(sps) > 0 {
						if spsInfo, err := h264parser.ParseSPS(sps); err == nil {
							m.width = uint32(spsInfo.Width)
							m.height = uint32(spsInfo.Height)
							m.fps = float64(spsInfo.FPS)
						}
					}
				}
			case av.H265:
				if h265CD, ok := cd.(h265parser.CodecData); ok {
					// Extract width/height from h265 codec data
					// This is simplified - real implementation would parse SPS
					_ = h265CD // Use variable to avoid warning
				m.width = 1920  // Default values
					m.height = 1080
					m.fps = 25
				}
			}
		} else if cd.Type().IsAudio() {
			m.audioStreamIdx = i
			m.hasAudio = true
			// Extract audio parameters
			switch cd.Type() {
			case av.AAC:
				if aacCD, ok := cd.(aacparser.CodecData); ok {
					m.audioSampleRate = uint32(aacCD.SampleRate())
				}
			case av.PCM_MULAW, av.PCM_ALAW:
				m.audioSampleRate = 8000 // Default for PCM codecs
			}
		}
	}

	if m.fps == 0 {
		m.fps = 25 // Default FPS
	}

	// Write headers
	return m.writeFileHeaders()
}

func (m *Muxer) writeFileHeaders() error {
	// Save position for later update
	var err error
	m.headerPos, err = m.ws.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}

	// Write RIFF header (will update size later)
	if err := aviio.WriteChunkHeader(m.ws, aviio.FourCCRIFF, 0); err != nil {
		return err
	}

	// Write AVI signature
	if err := binary.Write(m.ws, binary.LittleEndian, aviio.FourCCAVI); err != nil {
		return err
	}

	// Write header list
	if err := m.writeHeaderList(); err != nil {
		return err
	}

	// Write movi list header
	m.moviListPos, err = m.ws.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	if err := aviio.WriteChunkHeader(m.ws, aviio.FourCCLIST, 0); err != nil {
		return err
	}
	if err := binary.Write(m.ws, binary.LittleEndian, aviio.FourCCmovi); err != nil {
		return err
	}

	return nil
}

func (m *Muxer) writeHeaderList() error {
	// Calculate header list size
	headerBuf := &bytes.Buffer{}

	// Write to buffer first to calculate size
	if err := m.writeMainHeader(headerBuf); err != nil {
		return err
	}

	streamCount := 0
	if m.hasVideo {
		if err := m.writeStreamHeaders(headerBuf, m.videoStreamIdx, true); err != nil {
			return err
		}
		streamCount++
	}
	if m.hasAudio {
		if err := m.writeStreamHeaders(headerBuf, m.audioStreamIdx, false); err != nil {
			return err
		}
		streamCount++
	}

	// Write LIST header
	if err := aviio.WriteChunkHeader(m.ws, aviio.FourCCLIST, uint32(headerBuf.Len()+4)); err != nil {
		return err
	}
	if err := binary.Write(m.ws, binary.LittleEndian, aviio.FourCChdrl); err != nil {
		return err
	}

	// Write buffered content
	_, err := m.ws.Write(headerBuf.Bytes())
	return err
}

func (m *Muxer) writeMainHeader(w io.Writer) error {
	mainHeader := &aviio.MainAVIHeader{
		MicroSecPerFrame:    uint32(1000000 / m.fps),
		MaxBytesPerSec:      0, // Will be updated later
		PaddingGranularity:  0,
		Flags:               0x10, // AVIF_HASINDEX
		TotalFrames:         0,    // Will be updated later
		InitialFrames:       0,
		Streams:             uint32(len(m.codecData)),
		SuggestedBufferSize: 1048576,
		Width:               m.width,
		Height:              m.height,
	}

	if err := aviio.WriteChunkHeader(w, aviio.FourCCavih, 56); err != nil {
		return err
	}
	return aviio.WriteMainAVIHeader(w, mainHeader)
}

func (m *Muxer) writeStreamHeaders(w io.Writer, streamIdx int, isVideo bool) error {
	// Calculate stream header list size
	streamBuf := &bytes.Buffer{}

	if isVideo {
		if err := m.writeVideoStreamHeader(streamBuf, streamIdx); err != nil {
			return err
		}
	} else {
		if err := m.writeAudioStreamHeader(streamBuf, streamIdx); err != nil {
			return err
		}
	}

	// Write stream LIST
	if err := aviio.WriteChunkHeader(w, aviio.FourCCLIST, uint32(streamBuf.Len()+4)); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, aviio.FourCCstrl); err != nil {
		return err
	}

	_, err := w.Write(streamBuf.Bytes())
	return err
}

func (m *Muxer) writeVideoStreamHeader(w io.Writer, streamIdx int) error {
	codec := m.codecData[streamIdx]

	// Determine handler based on codec
	var handler uint32
	switch codec.Type() {
	case av.H264:
		handler = aviio.FourCC("H264")
	case av.H265:
		handler = aviio.FourCC("H265")
	default:
		handler = 0
	}

	streamHeader := &aviio.StreamHeader{
		Type:                aviio.FourCCvids,
		Handler:             handler,
		Scale:               1,
		Rate:                uint32(m.fps),
		SuggestedBufferSize: 1048576,
		Quality:             10000,
		SampleSize:          0, // Variable size
		Frame:               [4]uint16{0, 0, uint16(m.width), uint16(m.height)},
	}

	// Write stream header
	if err := aviio.WriteChunkHeader(w, aviio.FourCCstrh, 56); err != nil {
		return err
	}
	if err := aviio.WriteStreamHeader(w, streamHeader); err != nil {
		return err
	}

	// Write stream format
	return m.writeVideoFormat(w, codec)
}

func (m *Muxer) writeAudioStreamHeader(w io.Writer, streamIdx int) error {
	codec := m.codecData[streamIdx]

	streamHeader := &aviio.StreamHeader{
		Type:                aviio.FourCCauds,
		Scale:               1,
		Rate:                m.audioSampleRate,
		SuggestedBufferSize: 65536,
		Quality:             10000,
		SampleSize:          0, // Variable size
	}

	// Write stream header
	if err := aviio.WriteChunkHeader(w, aviio.FourCCstrh, 56); err != nil {
		return err
	}
	if err := aviio.WriteStreamHeader(w, streamHeader); err != nil {
		return err
	}

	// Write stream format
	return m.writeAudioFormat(w, codec)
}

func (m *Muxer) writeVideoFormat(w io.Writer, codec av.CodecData) error {
	bih := &aviio.BitmapInfoHeader{
		Size:          40,
		Width:         int32(m.width),
		Height:        int32(m.height),
		Planes:        1,
		BitCount:      24,
		SizeImage:     m.width * m.height * 3,
		XPelsPerMeter: 0,
		YPelsPerMeter: 0,
		ClrUsed:       0,
		ClrImportant:  0,
	}

	// Set compression based on codec
	switch codec.Type() {
	case av.H264:
		bih.Compression = aviio.FourCC("H264")
	case av.H265:
		bih.Compression = aviio.FourCC("H265")
	}

	// Calculate extra data size
	var extraData []byte
	switch codec.Type() {
	case av.H264:
		if h264CD, ok := codec.(h264parser.CodecData); ok {
			extraData = h264CD.AVCDecoderConfRecordBytes()
		}
	case av.H265:
		if h265CD, ok := codec.(h265parser.CodecData); ok {
			extraData = h265CD.AVCDecoderConfRecordBytes()
		}
	}

	// Write format chunk
	formatSize := uint32(40 + len(extraData))
	if err := aviio.WriteChunkHeader(w, aviio.FourCCstrf, formatSize); err != nil {
		return err
	}
	if err := aviio.WriteBitmapInfoHeader(w, bih); err != nil {
		return err
	}
	if len(extraData) > 0 {
		if _, err := w.Write(extraData); err != nil {
			return err
		}
	}

	return nil
}

func (m *Muxer) writeAudioFormat(w io.Writer, codec av.CodecData) error {
	wfx := &aviio.WaveFormatEx{
		Channels:      2, // Stereo by default
		BitsPerSample: 16,
	}

	var extraData []byte

	// Set format based on codec
	switch codec.Type() {
	case av.AAC:
		wfx.FormatTag = 0xFF
		if aacCD, ok := codec.(aacparser.CodecData); ok {
			wfx.SamplesPerSec = uint32(aacCD.SampleRate())
			wfx.Channels = uint16(aacCD.ChannelLayout().Count())
			extraData = aacCD.MPEG4AudioConfigBytes()
		}
	case av.PCM_MULAW:
		wfx.FormatTag = 0x07
		wfx.SamplesPerSec = m.audioSampleRate
		wfx.BitsPerSample = 8
	case av.PCM_ALAW:
		wfx.FormatTag = 0x06
		wfx.SamplesPerSec = m.audioSampleRate
		wfx.BitsPerSample = 8
	}

	wfx.BlockAlign = wfx.Channels * wfx.BitsPerSample / 8
	wfx.AvgBytesPerSec = wfx.SamplesPerSec * uint32(wfx.BlockAlign)
	wfx.CbSize = uint16(len(extraData))

	// Write format chunk
	formatSize := uint32(18 + len(extraData))
	if err := aviio.WriteChunkHeader(w, aviio.FourCCstrf, formatSize); err != nil {
		return err
	}
	if err := aviio.WriteWaveFormatEx(w, wfx); err != nil {
		return err
	}
	if len(extraData) > 0 {
		if _, err := w.Write(extraData); err != nil {
			return err
		}
	}

	return nil
}

func (m *Muxer) WritePacket(pkt av.Packet) error {
	// Determine chunk ID based on stream and type
	var chunkID uint32
	streamNum := int(pkt.Idx)
	
	if streamNum == m.videoStreamIdx {
		// Video chunk
		if pkt.IsKeyFrame {
			chunkID = aviio.FourCC(fmt.Sprintf("%02ddc", streamNum))
		} else {
			chunkID = aviio.FourCC(fmt.Sprintf("%02ddc", streamNum))
		}
	} else if streamNum == m.audioStreamIdx {
		// Audio chunk
		chunkID = aviio.FourCC(fmt.Sprintf("%02dwb", streamNum))
	} else {
		return fmt.Errorf("invalid stream index: %d", streamNum)
	}

	// Record index entry
	currentPos, err := m.getPosition()
	if err != nil {
		return err
	}
	indexEntry := aviio.IndexEntry{
		ChunkID: chunkID,
		Flags:   0,
		Offset:  uint32(currentPos - m.moviListPos - 12), // -12 = LIST header (8) + "movi" (4)
		Size:    uint32(len(pkt.Data)),
	}

	if pkt.IsKeyFrame {
		indexEntry.Flags |= aviio.AVIIF_KEYFRAME
	}

	m.indexEntries = append(m.indexEntries, indexEntry)

	// Write chunk header
	if err := aviio.WriteChunkHeader(m.ws, chunkID, uint32(len(pkt.Data))); err != nil {
		return err
	}

	// Write data
	if _, err := m.ws.Write(pkt.Data); err != nil {
		return err
	}

	// Align to word boundary
	if len(pkt.Data)&1 == 1 {
		if _, err := m.ws.Write([]byte{0}); err != nil {
			return err
		}
	}

	m.dataSize += uint32(8 + len(pkt.Data))
	if len(pkt.Data)&1 == 1 {
		m.dataSize++
	}

	if streamNum == m.videoStreamIdx {
		m.frameCount++
	}

	return nil
}

func (m *Muxer) WriteTrailer() error {
	// Write index
	if err := m.writeIndex(); err != nil {
		return err
	}

	// Get current position for total size calculation
	currentPos, err := m.ws.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}

	// Update file size in RIFF header
	totalSize := currentPos - 8
	if err := m.updateUint32At(4, uint32(totalSize)); err != nil {
		return err
	}

	// Update movi list size
	moviSize := m.dataSize + 4
	if err := m.updateUint32At(m.moviListPos+4, moviSize); err != nil {
		return err
	}

	// Update frame count in main header
	if err := m.updateUint32At(m.headerPos+48, m.frameCount); err != nil {
		return err
	}

	// If using WriterSeeker wrapper, flush to underlying writer
	if flusher, ok := m.ws.(interface{ Flush() error }); ok {
		return flusher.Flush()
	}

	return nil
}

func (m *Muxer) writeIndex() error {
	// Write idx1 chunk
	indexSize := uint32(len(m.indexEntries) * 16)
	if err := aviio.WriteChunkHeader(m.ws, aviio.FourCCidx1, indexSize); err != nil {
		return err
	}

	// Write all index entries
	for _, entry := range m.indexEntries {
		if err := binary.Write(m.ws, binary.LittleEndian, &entry); err != nil {
			return err
		}
	}

	return nil
}

func (m *Muxer) getPosition() (int64, error) {
	return m.ws.Seek(0, io.SeekCurrent)
}

func (m *Muxer) updateUint32At(offset int64, value uint32) error {
	// Save current position
	currentPos, err := m.ws.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}

	// Seek to offset
	if _, err := m.ws.Seek(offset, io.SeekStart); err != nil {
		return err
	}

	// Write value
	if err := binary.Write(m.ws, binary.LittleEndian, value); err != nil {
		return err
	}

	// Restore position
	_, err = m.ws.Seek(currentPos, io.SeekStart)
	return err
}