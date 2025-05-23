package openh264

// #include <string.h>
// #include <openh264/codec_api.h>
// #include <errno.h>
// #include "bridge.hpp"
import "C"

import (
	"fmt"
	"image"
	"io"
	"sync"
	"unsafe"

	"github.com/pion/mediadevices/pkg/codec"
	"github.com/pion/mediadevices/pkg/io/video"
	"github.com/pion/mediadevices/pkg/prop"
)

type encoder struct {
	engine *C.Encoder
	r      video.Reader

	mu     sync.Mutex
	closed bool
}

func newEncoder(r video.Reader, p prop.Media, params Params) (codec.ReadCloser, error) {
	if params.BitRate == 0 {
		params.BitRate = 100000
	}

	var rv C.int
	cEncoder := C.enc_new(C.EncoderOptions{
		width:                 C.int(p.Width),
		height:                C.int(p.Height),
		target_bitrate:        C.int(params.BitRate),
		max_fps:               C.float(p.FrameRate),
		usage_type:            C.EUsageType(params.UsageType),
		rc_mode:               C.RC_MODES(params.RCMode),
		enable_frame_skip:     C.bool(params.EnableFrameSkip),
		max_nal_size:          C.uint(params.MaxNalSize),
		intra_period:          C.uint(params.IntraPeriod),
		multiple_thread_idc:   C.int(params.MultipleThreadIdc),
		slice_num:             C.uint(params.SliceNum),
		slice_mode:            C.SliceModeEnum(params.SliceMode),
		slice_size_constraint: C.uint(params.SliceSizeConstraint),
	}, &rv)
	if err := errResult(rv); err != nil {
		return nil, fmt.Errorf("failed in creating encoder: %v", err)
	}

	return &encoder{
		engine: cEncoder,
		r:      video.ToI420(r),
	}, nil
}

func (e *encoder) Read() ([]byte, func(), error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil, func() {}, io.EOF
	}

	img, release, err := e.r.Read()
	if err != nil {
		return nil, func() {}, err
	}
	defer release()

	yuvImg := img.(*image.YCbCr)
	bounds := yuvImg.Bounds()
	var rv C.int
	s := C.enc_encode(e.engine, C.Frame{
		y:       unsafe.Pointer(&yuvImg.Y[0]),
		u:       unsafe.Pointer(&yuvImg.Cb[0]),
		v:       unsafe.Pointer(&yuvImg.Cr[0]),
		ystride: C.int(yuvImg.YStride),
		cstride: C.int(yuvImg.CStride),
		height:  C.int(bounds.Max.Y - bounds.Min.Y),
		width:   C.int(bounds.Max.X - bounds.Min.X),
	}, &rv)
	if err := errResult(rv); err != nil {
		return nil, func() {}, fmt.Errorf("failed in encoding: %v", err)
	}

	encoded := C.GoBytes(unsafe.Pointer(s.data), s.data_len)
	return encoded, func() {}, nil
}

func (e *encoder) ForceKeyFrame() error {
	e.engine.force_key_frame = C.int(1)
	return nil
}

func (e *encoder) SetBitRate(bitrate int) error {
	C.enc_set_bitrate(e.engine, C.int(bitrate))
	return nil
}

func (e *encoder) Controller() codec.EncoderController {
	return e
}

func (e *encoder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil
	}

	e.closed = true

	var rv C.int
	C.enc_free(e.engine, &rv)
	return errResult(rv)
}
