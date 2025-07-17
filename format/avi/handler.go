package avi

import (
	"bytes"
	"io"

	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/av/avutil"
)

func Handler(h *avutil.RegisterHandler) {
	h.Ext = ".avi"
	h.ReaderDemuxer = func(r io.Reader) av.Demuxer {
		return NewDemuxer(r)
	}
	h.WriterMuxer = func(w io.Writer) av.Muxer {
		if ws, ok := w.(io.WriteSeeker); ok {
			return NewMuxer(ws)
		}
		// For writers that don't support seeking, use a buffer wrapper
		return NewMuxer(NewWriterSeeker(w))
	}
	h.Probe = func(b []byte) bool {
		return detectAVIFormat(b)
	}
	h.CodecTypes = []av.CodecType{av.H264, av.H265, av.AAC, av.PCM_MULAW, av.PCM_ALAW}
}

// WriterSeeker wraps io.Writer to provide seeking functionality using a buffer
type WriterSeeker struct {
	w   io.Writer
	buf *bytes.Buffer
	pos int64
}

func NewWriterSeeker(w io.Writer) *WriterSeeker {
	return &WriterSeeker{
		w:   w,
		buf: &bytes.Buffer{},
	}
}

func (ws *WriterSeeker) Write(p []byte) (int, error) {
	// Handle seeking within buffer for header updates
	if ws.pos != int64(ws.buf.Len()) {
		data := ws.buf.Bytes()
		if ws.pos+int64(len(p)) <= int64(len(data)) {
			// Overwrite existing data
			copy(data[ws.pos:], p)
			ws.pos += int64(len(p))
			return len(p), nil
		}
	}
	n, err := ws.buf.Write(p)
	ws.pos += int64(n)
	return n, err
}

func (ws *WriterSeeker) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		ws.pos = offset
	case io.SeekCurrent:
		ws.pos += offset
	case io.SeekEnd:
		ws.pos = int64(ws.buf.Len()) + offset
	}
	return ws.pos, nil
}

// Flush writes buffered data to the underlying writer
func (ws *WriterSeeker) Flush() error {
	_, err := ws.w.Write(ws.buf.Bytes())
	return err
}

func detectAVIFormat(b []byte) bool {
	if len(b) < 12 {
		return false
	}
	// AVI files start with "RIFF" followed by file size and "AVI " 
	return string(b[0:4]) == "RIFF" && string(b[8:12]) == "AVI "
}