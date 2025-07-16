package avi

import (
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
		return NewMuxer(w)
	}
	h.Probe = func(b []byte) bool {
		return detectAVIFormat(b)
	}
	h.CodecTypes = []av.CodecType{av.H264, av.H265, av.AAC, av.PCM_MULAW, av.PCM_ALAW}
}

func detectAVIFormat(b []byte) bool {
	if len(b) < 12 {
		return false
	}
	// AVI files start with "RIFF" followed by file size and "AVI " 
	return string(b[0:4]) == "RIFF" && string(b[8:12]) == "AVI "
}