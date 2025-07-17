package avi

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
	"time"

	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/format/avi/aviio"
)

// BytesWriteSeeker wraps bytes.Buffer to implement io.WriteSeeker
type BytesWriteSeeker struct {
	*bytes.Buffer
	pos int64
}

func NewBytesWriteSeeker(buf *bytes.Buffer) *BytesWriteSeeker {
	return &BytesWriteSeeker{Buffer: buf}
}

func (bws *BytesWriteSeeker) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		bws.pos = offset
	case io.SeekCurrent:
		bws.pos += offset
	case io.SeekEnd:
		bws.pos = int64(bws.Buffer.Len()) + offset
	}
	return bws.pos, nil
}

func (bws *BytesWriteSeeker) Write(p []byte) (int, error) {
	// For simplicity, only support appending writes at the end
	if bws.pos != int64(bws.Buffer.Len()) {
		// Need to handle seeking within buffer for header updates
		currentData := bws.Buffer.Bytes()
		if bws.pos+int64(len(p)) <= int64(len(currentData)) {
			// Overwrite existing data
			copy(currentData[bws.pos:], p)
			bws.pos += int64(len(p))
			return len(p), nil
		}
	}
	n, err := bws.Buffer.Write(p)
	bws.pos += int64(n)
	return n, err
}

// BytesReadSeeker wraps bytes.Buffer to implement io.ReadSeeker
type BytesReadSeeker struct {
	*bytes.Reader
}

func NewBytesReadSeeker(buf *bytes.Buffer) *BytesReadSeeker {
	return &BytesReadSeeker{Reader: bytes.NewReader(buf.Bytes())}
}

// Test helper functions

// createTestAVIFile creates a complete AVI file with video chunks for testing
func createTestAVIFile() *bytes.Buffer {
	buf := &bytes.Buffer{}
	ws := NewBytesWriteSeeker(buf)
	
	// Create muxer and write a proper AVI file
	muxer := NewMuxer(ws)
	
	// Create H.264 codec data
	codecData := []av.CodecData{
		&h264parser.CodecData{},
	}
	
	// Write header
	if err := muxer.WriteHeader(codecData); err != nil {
		panic(err)
	}
	
	// Write multiple test packets with different H.264 NAL units
	testFrames := []struct {
		data       []byte
		isKeyFrame bool
	}{
		// SPS NAL unit (keyframe)
		{[]byte{0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0x00, 0x1f, 0x8b, 0x68, 0x42, 0x01}, true},
		// PPS NAL unit  
		{[]byte{0x00, 0x00, 0x00, 0x01, 0x68, 0xce, 0x38, 0x80}, false},
		// I-frame NAL unit (keyframe)
		{[]byte{0x00, 0x00, 0x00, 0x01, 0x65, 0x88, 0x84, 0x00, 0x33, 0xff, 0xe4, 0x93}, true},
		// P-frame NAL unit
		{[]byte{0x00, 0x00, 0x00, 0x01, 0x61, 0xe1, 0x05, 0x17, 0x33, 0xff}, false},
		// Another P-frame
		{[]byte{0x00, 0x00, 0x00, 0x01, 0x61, 0xe1, 0x05, 0x18, 0x44, 0xaa}, false},
	}
	
	for i, frame := range testFrames {
		packet := av.Packet{
			Idx:        0,
			IsKeyFrame: frame.isKeyFrame,
			Time:       time.Duration(i) * time.Second / 25, // 25 FPS (default)
			Data:       frame.data,
		}
		
		if err := muxer.WritePacket(packet); err != nil {
			panic(err)
		}
	}
	
	// Write trailer
	if err := muxer.WriteTrailer(); err != nil {
		panic(err)
	}
	
	return buf
}

// createTestAVIHeader creates a minimal valid AVI header for testing
func createTestAVIHeader() []byte {
	buf := bytes.NewBuffer(nil)

	// RIFF header
	buf.Write([]byte("RIFF"))
	binary.Write(buf, binary.LittleEndian, uint32(1000)) // File size - 8
	buf.Write([]byte("AVI "))

	// LIST hdrl
	buf.Write([]byte("LIST"))
	binary.Write(buf, binary.LittleEndian, uint32(200)) // hdrl size
	buf.Write([]byte("hdrl"))

	// avih chunk
	buf.Write([]byte("avih"))
	binary.Write(buf, binary.LittleEndian, uint32(56)) // avih size

	// AVI main header
	binary.Write(buf, binary.LittleEndian, uint32(33333))   // MicroSecPerFrame (30 fps)
	binary.Write(buf, binary.LittleEndian, uint32(1000000)) // MaxBytesPerSec
	binary.Write(buf, binary.LittleEndian, uint32(0))       // PaddingGranularity
	binary.Write(buf, binary.LittleEndian, uint32(0))       // Flags
	binary.Write(buf, binary.LittleEndian, uint32(300))     // TotalFrames
	binary.Write(buf, binary.LittleEndian, uint32(0))       // InitialFrames
	binary.Write(buf, binary.LittleEndian, uint32(1))       // Streams
	binary.Write(buf, binary.LittleEndian, uint32(0))       // SuggestedBufferSize
	binary.Write(buf, binary.LittleEndian, uint32(640))     // Width
	binary.Write(buf, binary.LittleEndian, uint32(480))     // Height
	// Reserved fields
	for i := 0; i < 4; i++ {
		binary.Write(buf, binary.LittleEndian, uint32(0))
	}

	return buf.Bytes()
}

// Unit Tests

func TestAVIDetection(t *testing.T) {
	// Test AVI format detection
	aviHeader := []byte("RIFF\x00\x00\x00\x00AVI LIST")

	if !detectAVIFormat(aviHeader) {
		t.Error("Failed to detect AVI format")
	}

	// Test non-AVI data
	wrongHeader := []byte("NOT_AVI_FORMAT")
	if detectAVIFormat(wrongHeader) {
		t.Error("False positive detection for non-AVI data")
	}
	
	// Test insufficient data
	shortHeader := []byte("RIFF")
	if detectAVIFormat(shortHeader) {
		t.Error("Should not detect AVI with insufficient data")
	}
}

func TestAVIIOStructures(t *testing.T) {
	// Test FourCC conversion
	fourcc := aviio.FourCC("H264")
	if aviio.FourCCString(fourcc) != "H264" {
		t.Error("FourCC conversion failed")
	}
	
	// Test main header parsing
	headerData := createTestAVIHeader()[20:] // Skip RIFF header to get to LIST
	// Parse the LIST chunk manually to get to avih
	if len(headerData) >= 24 && string(headerData[8:12]) == "avih" {
		avihData := headerData[16:72] // Skip LIST header and avih chunk header
		header, err := aviio.ReadMainAVIHeader(bytes.NewReader(avihData))
		if err != nil {
			t.Fatalf("Failed to parse main header: %v", err)
		}
		
		if header.MicroSecPerFrame != 33333 {
			t.Errorf("Expected MicroSecPerFrame=33333, got %d", header.MicroSecPerFrame)
		}
		
		if header.Width != 640 {
			t.Errorf("Expected Width=640, got %d", header.Width)
		}
		
		if header.Height != 480 {
			t.Errorf("Expected Height=480, got %d", header.Height)
		}
	}
}

func TestMuxerDemuxerRoundTrip(t *testing.T) {
	// Create a complete AVI file
	aviData := createTestAVIFile()
	
	t.Logf("Created AVI file of size: %d bytes", aviData.Len())
	
	// Test demuxer with the created file
	demuxer := NewDemuxer(NewBytesReadSeeker(aviData))
	
	// Test stream parsing
	streams, err := demuxer.Streams()
	if err != nil {
		t.Fatalf("Failed to read streams: %v", err)
	}
	
	if len(streams) == 0 {
		t.Fatal("No streams found")
	}
	
	t.Logf("Found %d streams", len(streams))
	
	// Check stream type
	if streams[0].Type() != av.H264 {
		t.Errorf("Expected H264 stream, got %v", streams[0].Type())
	}
	
	// Read all packets
	packetCount := 0
	keyFrameCount := 0
	
	for {
		packet, err := demuxer.ReadPacket()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Failed to read packet %d: %v", packetCount, err)
		}
		
		packetCount++
		if packet.IsKeyFrame {
			keyFrameCount++
		}
		
		// Verify packet data
		if len(packet.Data) == 0 {
			t.Errorf("Packet %d has no data", packetCount)
		}
		
		// Check for H.264 start codes
		if len(packet.Data) >= 4 {
			if packet.Data[0] == 0x00 && packet.Data[1] == 0x00 && 
			   packet.Data[2] == 0x00 && packet.Data[3] == 0x01 {
				t.Logf("Packet %d: Valid H.264 NAL unit, size=%d, keyframe=%v", 
					packetCount, len(packet.Data), packet.IsKeyFrame)
			}
		}
		
		// Verify stream index
		if packet.Idx != 0 {
			t.Errorf("Expected stream index 0, got %d", packet.Idx)
		}
	}
	
	t.Logf("Successfully read %d packets (%d keyframes)", packetCount, keyFrameCount)
	
	if packetCount == 0 {
		t.Error("No packets were read")
	}
	
	if keyFrameCount == 0 {
		t.Error("No keyframes were detected")
	}
}

func TestDemuxerEdgeCases(t *testing.T) {
	// Test with invalid AVI data
	invalidData := []byte("NOT_A_VALID_AVI_FILE")
	demuxer := NewDemuxer(bytes.NewReader(invalidData))
	
	_, err := demuxer.Streams()
	if err == nil {
		t.Error("Expected error for invalid AVI data")
	}
	
	// Test with empty data
	emptyDemuxer := NewDemuxer(bytes.NewReader([]byte{}))
	_, err = emptyDemuxer.Streams()
	if err == nil {
		t.Error("Expected error for empty data")
	}
}

func TestChunkAlignment(t *testing.T) {
	// Test that odd-sized chunks are properly aligned
	testData := []byte{0x01, 0x02, 0x03} // 3 bytes, odd size

	buf := &bytes.Buffer{}
	muxer := NewMuxer(NewBytesWriteSeeker(buf))

	codecData := []av.CodecData{
		&h264parser.CodecData{},
	}

	err := muxer.WriteHeader(codecData)
	if err != nil {
		t.Fatalf("Failed to write header: %v", err)
	}

	packet := av.Packet{
		Idx:  0,
		Data: testData,
	}

	err = muxer.WritePacket(packet)
	if err != nil {
		t.Fatalf("Failed to write packet: %v", err)
	}
	
	err = muxer.WriteTrailer()
	if err != nil {
		t.Fatalf("Failed to write trailer: %v", err)
	}

	// Test that the file can be read back
	demuxer := NewDemuxer(NewBytesReadSeeker(buf))
	_, err = demuxer.Streams()
	if err != nil {
		t.Fatalf("Failed to read back aligned chunks: %v", err)
	}
}

func TestStreamHeaderParsing(t *testing.T) {
	// Create a proper stream header
	data := make([]byte, 56)
	binary.LittleEndian.PutUint32(data[0:4], aviio.FourCCvids) // Type: video
	binary.LittleEndian.PutUint32(data[4:8], aviio.FourCC("H264")) // Handler: H264
	binary.LittleEndian.PutUint32(data[20:24], 1)   // Scale
	binary.LittleEndian.PutUint32(data[24:28], 30)  // Rate (30 fps)
	binary.LittleEndian.PutUint32(data[32:36], 300) // Length

	header, err := aviio.ReadStreamHeader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to parse stream header: %v", err)
	}

	if header.Type != aviio.FourCCvids {
		t.Errorf("Expected Type=vids, got %s", aviio.FourCCString(header.Type))
	}

	if header.Handler != aviio.FourCC("H264") {
		t.Errorf("Expected Handler=H264, got %s", aviio.FourCCString(header.Handler))
	}

	if header.Scale != 1 {
		t.Errorf("Expected Scale=1, got %d", header.Scale)
	}

	if header.Rate != 30 {
		t.Errorf("Expected Rate=30, got %d", header.Rate)
	}
}

func TestIndexParsing(t *testing.T) {
	// Create a test index with multiple entries
	buf := &bytes.Buffer{}
	
	// Write 3 index entries
	for i := 0; i < 3; i++ {
		entry := aviio.IndexEntry{
			ChunkID: aviio.FourCC("00dc"),
			Flags:   aviio.AVIIF_KEYFRAME,
			Offset:  uint32(i * 1000),
			Size:    uint32(500 + i*100),
		}
		binary.Write(buf, binary.LittleEndian, &entry)
	}
	
	// Parse entries
	data := buf.Bytes()
	numEntries := len(data) / 16
	
	if numEntries != 3 {
		t.Errorf("Expected 3 entries, calculated %d", numEntries)
	}
	
	// Read back entries
	reader := bytes.NewReader(data)
	for i := 0; i < numEntries; i++ {
		var entry aviio.IndexEntry
		err := binary.Read(reader, binary.LittleEndian, &entry)
		if err != nil {
			t.Fatalf("Failed to read entry %d: %v", i, err)
		}
		
		if entry.ChunkID != aviio.FourCC("00dc") {
			t.Errorf("Entry %d: expected ChunkID=00dc, got %s", i, aviio.FourCCString(entry.ChunkID))
		}
		
		if entry.Offset != uint32(i*1000) {
			t.Errorf("Entry %d: expected offset=%d, got %d", i, i*1000, entry.Offset)
		}
	}
}

func TestTimestampCalculation(t *testing.T) {
	// Create AVI file with known frame rate
	aviData := createTestAVIFile()
	demuxer := NewDemuxer(NewBytesReadSeeker(aviData))
	
	// Get streams
	_, err := demuxer.Streams()
	if err != nil {
		t.Fatalf("Failed to read streams: %v", err)
	}
	
	// Read first few packets and check timestamps
	for i := 0; i < 3; i++ {
		packet, err := demuxer.ReadPacket()
		if err != nil {
			t.Fatalf("Failed to read packet %d: %v", i, err)
		}
		
		expectedTime := time.Duration(i) * time.Second / 25 // 25 FPS (default)
		if packet.Time != expectedTime {
			t.Errorf("Packet %d: expected time=%v, got %v", i, expectedTime, packet.Time)
		}
	}
}

// Benchmark tests

func BenchmarkDemuxerParsing(b *testing.B) {
	aviData := createTestAVIFile()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		demuxer := NewDemuxer(NewBytesReadSeeker(aviData))
		_, err := demuxer.Streams()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPacketReading(b *testing.B) {
	aviData := createTestAVIFile()
	demuxer := NewDemuxer(NewBytesReadSeeker(aviData))
	_, err := demuxer.Streams()
	if err != nil {
		b.Fatal(err)
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Reset demuxer position
		demuxer.currentFrame = 0
		
		// Read first packet
		_, err := demuxer.ReadPacket()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMuxerWriting(b *testing.B) {
	codecData := []av.CodecData{
		&h264parser.CodecData{},
	}
	
	testPacket := av.Packet{
		Idx:        0,
		IsKeyFrame: true,
		Data:       []byte{0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0x00, 0x1f},
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := &bytes.Buffer{}
		muxer := NewMuxer(NewBytesWriteSeeker(buf))
		
		muxer.WriteHeader(codecData)
		muxer.WritePacket(testPacket)
		muxer.WriteTrailer()
	}
}