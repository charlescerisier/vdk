# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

VDK (Video Development Kit) is a Go-based streaming library forked from JOY4, designed for building video streaming services. It provides comprehensive support for multiple video/audio formats, codecs, and streaming protocols.

## Development Commands

### Build and Test
```bash
# Build all packages
go build ./...

# Run tests
go test ./...

# Run tests for a specific package
go test ./format/rtmp/...

# Download dependencies
go mod download

# Fix missing dependency issue
go get github.com/shirou/gopsutil/v3/disk
```

### FFmpeg Integration (Optional)
For CGO components that integrate with FFmpeg:
```bash
# Ensure FFmpeg development headers are installed
# macOS: brew install ffmpeg
# Ubuntu: sudo apt-get install libavcodec-dev libavformat-dev libavutil-dev libswscale-dev
```

## Architecture

### Core Components

1. **AV Core (`/av/`)**: Central abstractions for audio/video handling
   - `av.go`: Core interfaces (Demuxer, Muxer, Packet, etc.)
   - `pubsub/`: Implements publish/subscribe pattern for stream distribution
   - `pktque/`: Packet buffering and queue management
   - `transcode/`: Transcoding pipeline implementation

2. **Codecs (`/codec/`)**: Parser implementations for various codecs
   - Each codec has its own package (h264parser, h265parser, aacparser, etc.)
   - Parsers extract codec-specific information from raw streams

3. **Formats (`/format/`)**: Container format and protocol implementations
   - Each format/protocol is self-contained in its own package
   - Implements Demuxer/Muxer interfaces from av package

### Key Design Patterns

1. **Handler Registration**: Formats register themselves via `format.RegisterAll()`
2. **Interface-Based**: Heavy use of interfaces for flexibility (Demuxer, Muxer, CodecData)
3. **Stream Pipeline**: Demuxer → Packet Queue → Muxer architecture
4. **Packet-Based**: All data flows through av.Packet structures

### Adding New Features

To add a new format:
1. Create package in `/format/yourformat/`
2. Implement `av.Demuxer` and/or `av.Muxer` interfaces
3. Register in `format.RegisterAll()`

To add a new codec parser:
1. Create package in `/codec/yourcodec/`
2. Implement parser following existing patterns
3. Handle SPS/PPS extraction for video codecs

## Common Issues and Solutions

### Build/Test Failures
- **Missing gopsutil**: Run `go get github.com/shirou/gopsutil/v3/disk`
- **FFmpeg headers not found**: Install FFmpeg development packages or skip CGO tests
- **Format string errors**: Fix in `av/avutil/avutil.go:77` (add format arguments)
- **Test compilation errors**: Fix in `codec/h264parser/parser_test.go` (slice type mismatch)

### Protocol-Specific Notes
- **RTSP**: Two implementations (rtsp and rtspv2), rtspv2 is newer
- **WebRTC**: Two versions (webrtc and webrtcv3), webrtcv3 uses newer Pion libraries
- **HLS**: Supports both muxing and demuxing
- **RTMP**: Full client/server implementation

## Testing Approach

1. **Unit Tests**: Test individual codec parsers and utilities
2. **Integration Tests**: Test format handlers with sample streams
3. **Example Programs**: Use `/example/` directory for end-to-end testing
4. **Transcoder Example**: `/example/transcoder/` shows RTSP → FFmpeg pipeline

## Code Organization Tips

- Keep format-specific code isolated in format packages
- Use av.Packet for all data transfer between components
- Follow existing error handling patterns (return early on error)
- Maintain clean interfaces between layers