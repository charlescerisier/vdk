package avi

import (
	"bytes"
	"testing"

	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/codec/h264parser"
)

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
}

func TestMuxerDemuxer(t *testing.T) {
	// Create a buffer to write/read AVI data
	buf := &bytes.Buffer{}

	// Create muxer
	muxer := NewMuxer(buf)

	// Create mock codec data (simplified for testing)
	codecData := []av.CodecData{
		&h264parser.CodecData{},
	}

	// Test WriteHeader
	err := muxer.WriteHeader(codecData)
	if err != nil {
		t.Fatalf("Failed to write header: %v", err)
	}

	// Create a test packet
	packet := av.Packet{
		Idx:        0,
		IsKeyFrame: true,
		Data:       []byte{0x00, 0x00, 0x00, 0x01, 0x67}, // H.264 SPS NAL
	}

	// Test WritePacket
	err = muxer.WritePacket(packet)
	if err != nil {
		t.Fatalf("Failed to write packet: %v", err)
	}

	// Test WriteTrailer
	err = muxer.WriteTrailer()
	if err != nil {
		t.Fatalf("Failed to write trailer: %v", err)
	}

	// Test demuxer
	demuxer := NewDemuxer(buf)

	// Test Streams
	streams, err := demuxer.Streams()
	if err != nil {
		t.Fatalf("Failed to read streams: %v", err)
	}

	if len(streams) == 0 {
		t.Error("No streams found")
	}

	t.Logf("Successfully created and tested AVI muxer/demuxer with %d streams", len(streams))
}

func TestChunkAlignment(t *testing.T) {
	// Test that odd-sized chunks are properly aligned
	testData := []byte{0x01, 0x02, 0x03} // 3 bytes, odd size
	
	buf := &bytes.Buffer{}
	muxer := NewMuxer(buf)
	
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
	
	// The muxer should have added padding for alignment
	if buf.Len()%2 != 0 {
		t.Error("Buffer should be word-aligned after writing odd-sized packet")
	}
}