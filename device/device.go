package device

import (
	"context"
	"fmt"
	"os"
	"reflect"
	sys "syscall"

	"github.com/Java2019/go4vlv1/v4l2"
)

type Device struct {
	path         string
	file         *os.File
	fd           uintptr
	config       config
	bufType      v4l2.BufType
	cap          v4l2.Capability
	cropCap      v4l2.CropCapability
	buffers      [][]byte
	requestedBuf v4l2.RequestBuffers
	streaming    bool
	output       chan []byte
}

// Open creates opens the underlying device at specified path for streaming.
// It returns a *Device or an error if unable to open device.
func Open(path string, options ...Option) (*Device, error) {
	dev := &Device{path: path, config: config{}}
	// apply options
	if len(options) > 0 {
		for _, o := range options {
			o(&dev.config)
		}
	}

	file, err := os.OpenFile(path, sys.O_RDWR|sys.O_NONBLOCK, 0644)
	//file, err := os.OpenFile(path, sys.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("device open: %w", err)
	}
	dev.file = file
	dev.fd = file.Fd()

	// get capability
	cap, err := v4l2.GetCapability(file.Fd())
	if err != nil {
		if err := file.Close(); err != nil {
			return nil, fmt.Errorf("device %s: closing after failure: %s", path, err)
		}
		return nil, fmt.Errorf("device open: %s: %w", path, err)
	}
	dev.cap = cap

	// set preferred device buffer size
	if reflect.ValueOf(dev.config.bufSize).IsZero() {
		dev.config.bufSize = 2
	}

	// only supports streaming IO model right now
	if !dev.cap.IsStreamingSupported() {
		return nil, fmt.Errorf("device open: only streaming IO is supported")
	}

	switch {
	case cap.IsVideoCaptureSupported():
		// setup capture parameters and chan for captured data
		dev.bufType = v4l2.BufTypeVideoCapture
		dev.output = make(chan []byte, dev.config.bufSize)
	case cap.IsVideoOutputSupported():
		dev.bufType = v4l2.BufTypeVideoOutput
	default:
		if err := file.Close(); err != nil {
			return nil, fmt.Errorf("device open: %s: closing after failure: %s", path, err)
		}
		return nil, fmt.Errorf("device open: %s: %w", path, v4l2.ErrorUnsupportedFeature)
	}

	if !reflect.ValueOf(dev.config.bufType).IsZero() && dev.config.bufType != dev.bufType {
		return nil, fmt.Errorf("device open: does not support buffer stream type")
	}

	// ensures IOType is set
	if reflect.ValueOf(dev.config.ioType).IsZero() {
		dev.config.ioType = v4l2.IOTypeMMAP
	}

	// set pix format
	if !reflect.ValueOf(dev.config.pixFormat).IsZero() {
		if err := dev.SetPixFormat(dev.config.pixFormat); err != nil {
			fmt.Errorf("device open: %s: set format: %w", path, err)
		}
	} else {
		if dev.config.pixFormat, err = v4l2.GetPixFormat(dev.fd); err != nil {
			fmt.Errorf("device open: %s: get pix format: %w", path, err)
		}
	}

	// set fps
	if !reflect.ValueOf(dev.config.fps).IsZero() {
		if err := dev.SetFrameRate(dev.config.fps); err != nil {
			fmt.Errorf("device open: %s: set fps: %w", path, err)
		}
	} else {
		if dev.config.fps, err = dev.GetFrameRate(); err != nil {
			fmt.Errorf("device open: %s: get fps: %w", path, err)
		}
	}

	return dev, nil
}

// Close closes the underlying device associated with `d` .
func (d *Device) Close() error {
	if d.streaming {
		if err := d.Stop(); err != nil {
			return err
		}
	}

	return d.file.Close()
}

// Name returns the device name (or path)
func (d *Device) Name() string {
	return d.path
}

// Fd returns the file descriptor value for the device
func (d *Device) Fd() uintptr {
	return d.fd
}

// Buffers returns the internal mapped buffers. This method should be
// called after streaming has been started otherwise it may return nil.
func (d *Device) Buffers() [][]byte {
	return d.buffers
}

// Capability returns device capability info.
func (d *Device) Capability() v4l2.Capability {
	return d.cap
}

// BufferType this is a convenience method that returns the device mode (i.e. Capture, Output, etc)
// Use method Capability for detail about the device.
func (d *Device) BufferType() v4l2.BufType {
	return d.bufType
}

// BufferCount returns configured number of buffers to be used during streaming.
// If called after streaming start, this value could be updated by the driver.
func (d *Device) BufferCount() v4l2.BufType {
	return d.config.bufSize
}

// MemIOType returns the device memory input/output type (i.e. Memory mapped, DMA, user pointer, etc)
func (d *Device) MemIOType() v4l2.IOType {
	return d.config.ioType
}

// GetOutput returns the channel that outputs streamed data that is
// captured from the underlying device driver.
func (d *Device) GetOutput() <-chan []byte {
	return d.output
}

// SetInput sets up an input channel for data this sent for output to the
// underlying device driver.
func (d *Device) SetInput(in <-chan []byte) {

}

// GetCropCapability returns cropping info for device
func (d *Device) GetCropCapability() (v4l2.CropCapability, error) {
	if !d.cap.IsVideoCaptureSupported() {
		return v4l2.CropCapability{}, v4l2.ErrorUnsupportedFeature
	}
	return d.cropCap, nil
}

// SetCropRect crops the video dimension for the device
func (d *Device) SetCropRect(r v4l2.Rect) error {
	if !d.cap.IsVideoCaptureSupported() {
		return v4l2.ErrorUnsupportedFeature
	}
	if err := v4l2.SetCropRect(d.fd, r); err != nil {
		return fmt.Errorf("device: %w", err)
	}
	return nil
}

// GetPixFormat retrieves pixel format info for device
func (d *Device) GetPixFormat() (v4l2.PixFormat, error) {
	if !d.cap.IsVideoCaptureSupported() {
		return v4l2.PixFormat{}, v4l2.ErrorUnsupportedFeature
	}

	if reflect.ValueOf(d.config.pixFormat).IsZero() {
		pixFmt, err := v4l2.GetPixFormat(d.fd)
		if err != nil {
			return v4l2.PixFormat{}, fmt.Errorf("device: %w", err)
		}
		d.config.pixFormat = pixFmt
	}

	return d.config.pixFormat, nil
}

// SetPixFormat sets the pixel format for the associated device.
func (d *Device) SetPixFormat(pixFmt v4l2.PixFormat) error {
	if !d.cap.IsVideoCaptureSupported() {
		return v4l2.ErrorUnsupportedFeature
	}

	if err := v4l2.SetPixFormat(d.fd, pixFmt); err != nil {
		return fmt.Errorf("device: %w", err)
	}
	d.config.pixFormat = pixFmt
	return nil
}

// GetFormatDescription returns a format description for the device at specified format index
func (d *Device) GetFormatDescription(idx uint32) (v4l2.FormatDescription, error) {
	if !d.cap.IsVideoCaptureSupported() {
		return v4l2.FormatDescription{}, v4l2.ErrorUnsupportedFeature
	}

	return v4l2.GetFormatDescription(d.fd, idx)
}

// GetFormatDescriptions returns all possible format descriptions for device
func (d *Device) GetFormatDescriptions() ([]v4l2.FormatDescription, error) {
	if !d.cap.IsVideoCaptureSupported() {
		return nil, v4l2.ErrorUnsupportedFeature
	}

	return v4l2.GetAllFormatDescriptions(d.fd)
}

// GetVideoInputIndex returns current video input index for device
func (d *Device) GetVideoInputIndex() (int32, error) {
	if !d.cap.IsVideoCaptureSupported() {
		return 0, v4l2.ErrorUnsupportedFeature
	}

	return v4l2.GetCurrentVideoInputIndex(d.fd)
}

// GetVideoInputInfo returns video input info for device
func (d *Device) GetVideoInputInfo(index uint32) (v4l2.InputInfo, error) {
	if !d.cap.IsVideoCaptureSupported() {
		return v4l2.InputInfo{}, v4l2.ErrorUnsupportedFeature
	}

	return v4l2.GetVideoInputInfo(d.fd, index)
}

// GetStreamParam returns streaming parameter information for device
func (d *Device) GetStreamParam() (v4l2.StreamParam, error) {
	if !d.cap.IsVideoCaptureSupported() && d.cap.IsVideoOutputSupported() {
		return v4l2.StreamParam{}, v4l2.ErrorUnsupportedFeature
	}
	return v4l2.GetStreamParam(d.fd, d.bufType)
}

// SetStreamParam saves stream parameters for device
func (d *Device) SetStreamParam(param v4l2.StreamParam) error {
	if !d.cap.IsVideoCaptureSupported() && d.cap.IsVideoOutputSupported() {
		return v4l2.ErrorUnsupportedFeature
	}
	return v4l2.SetStreamParam(d.fd, d.bufType, param)
}

// SetFrameRate sets the FPS rate value of the device
func (d *Device) SetFrameRate(fps uint32) error {
	if !d.cap.IsStreamingSupported() {
		return fmt.Errorf("set frame rate: %w", v4l2.ErrorUnsupportedFeature)
	}

	var param v4l2.StreamParam
	switch {
	case d.cap.IsVideoCaptureSupported():
		param.Capture = v4l2.CaptureParam{TimePerFrame: v4l2.Fract{Numerator: 1, Denominator: fps}}
	case d.cap.IsVideoOutputSupported():
		param.Output = v4l2.OutputParam{TimePerFrame: v4l2.Fract{Numerator: 1, Denominator: fps}}
	default:
		return v4l2.ErrorUnsupportedFeature
	}
	if err := d.SetStreamParam(param); err != nil {
		return fmt.Errorf("device: set fps: %w", err)
	}
	d.config.fps = fps
	return nil
}

// GetFrameRate returns the FPS value for the device
func (d *Device) GetFrameRate() (uint32, error) {
	if reflect.ValueOf(d.config.fps).IsZero() {
		param, err := d.GetStreamParam()
		if err != nil {
			return 0, fmt.Errorf("device: frame rate: %w", err)
		}
		switch {
		case d.cap.IsVideoCaptureSupported():
			d.config.fps = param.Capture.TimePerFrame.Denominator
		case d.cap.IsVideoOutputSupported():
			d.config.fps = param.Output.TimePerFrame.Denominator
		default:
			return 0, v4l2.ErrorUnsupportedFeature
		}
	}

	return d.config.fps, nil
}

// GetMediaInfo returns info for a device that supports the Media API
func (d *Device) GetMediaInfo() (v4l2.MediaDeviceInfo, error) {
	return v4l2.GetMediaDeviceInfo(d.fd)
}

func (d *Device) Start(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if !d.cap.IsStreamingSupported() {
		return fmt.Errorf("device: start stream: %s", v4l2.ErrorUnsupportedFeature)
	}

	if d.streaming {
		return fmt.Errorf("device: stream already started")
	}

	// allocate device buffers
	bufReq, err := v4l2.InitBuffers(d)
	if err != nil {
		return fmt.Errorf("device: init buffers: %w", err)
	}
	d.config.bufSize = bufReq.Count // update with granted buf size
	d.requestedBuf = bufReq

	// for each allocated device buf, map into local space
	if d.buffers, err = v4l2.MapMemoryBuffers(d); err != nil {
		return fmt.Errorf("device: make mapped buffers: %s", err)
	}

	// Initial enqueue of buffers for capture
	for i := 0; i < int(d.config.bufSize); i++ {
		_, err := v4l2.QueueBuffer(d.fd, d.config.ioType, d.bufType, uint32(i))
		if err != nil {
			return fmt.Errorf("device: initial buffer queueing: %w", err)
		}
	}

	if err := d.startStreamLoop(ctx); err != nil {
		return fmt.Errorf("device: start stream loop: %s", err)
	}

	d.streaming = true

	return nil
}

func (d *Device) Stop() error {
	if !d.streaming {
		return nil
	}
	if err := v4l2.UnmapMemoryBuffers(d); err != nil {
		return fmt.Errorf("device: stop: %w", err)
	}
	if err := v4l2.StreamOff(d); err != nil {
		return fmt.Errorf("device: stop: %w", err)
	}
	d.streaming = false
	return nil
}

func (d *Device) startStreamLoop(ctx context.Context) error {
	if err := v4l2.StreamOn(d); err != nil {
		return fmt.Errorf("stream loop: stream on: %w", err)
	}

	go func() {
		defer close(d.output)

		fd := d.Fd()
		ioMemType := d.MemIOType()
		bufType := d.BufferType()

		for {
			select {
			// handle stream capture (read from driver)
			case <-v4l2.WaitForRead(d):
				//TODO add better error-handling, for now just panic
				buff, err := v4l2.CaptureBuffer(fd, ioMemType, bufType)
				if err != nil {
					panic(fmt.Errorf("stream loop: capture buffer: %s", err).Error())
				}

				d.output <- d.Buffers()[buff.Index][:buff.BytesUsed]

			case <-ctx.Done():
				d.Stop()
				return
			}
		}
	}()

	return nil
}
