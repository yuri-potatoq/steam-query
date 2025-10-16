package main

/*
To compile this module is necessary setup variables bellow

export CGO_CFLAGS=$(pkg-config --cflags libavformat)
export CGO_LDFLAGS=$(pkg-config --libs libavformat libavcodec libavutil)
*/

/*
   #include <libavformat/avformat.h>
*/
import "C"
import (
	"errors"
	"fmt"
	"unsafe"
)

func AVFormatVersion() {
	fmt.Printf("AV_FORMAT Version: %d\n", C.avformat_version())
}


// Should have the same purpose as these commands:
//
// ffmpeg -i video.m4s -i audio.m4s -c copy output.mp4
//
// ffmpeg -f mp4 -i video.m4s -c copy output.mp4
func TransformMedia(videoFile, audioFile, outputFile string) error {
	var (
		inVideoCtx *C.AVFormatContext
		inAudioCtx *C.AVFormatContext
		outCtx     *C.AVFormatContext
	)

	outputName := C.CString(outputFile)

	defer func() {
		C.avformat_close_input(&inVideoCtx)
		C.avformat_close_input(&inAudioCtx)

		C.free(unsafe.Pointer(outputName))
	}()

	if err := setupInputFile(videoFile, &inVideoCtx); err != nil {
		return err
	}
	if err := setupInputFile(audioFile, &inAudioCtx); err != nil {
		return err
	}

	if ret := C.avformat_alloc_output_context2(&outCtx, nil, nil, outputName); ret < 0 {
		return errors.New("can't create output context")
	}
	defer C.avformat_free_context(outCtx)

	outVideoStream := createAndSetupStream(inVideoCtx, outCtx)
	outAudioStream := createAndSetupStream(inAudioCtx, outCtx)

	if (outCtx.oformat.flags & C.AVFMT_NOFILE) == 0 {
		if C.avio_open(&outCtx.pb, outputName, C.AVIO_FLAG_WRITE) < 0 {
			return errors.New("could not open output file")
		}
	}

	C.avformat_write_header(outCtx, nil)

	var packet C.AVPacket

	copyStreamPackets(&packet, outVideoStream, inVideoCtx, outCtx)
	copyStreamPackets(&packet, outAudioStream, inAudioCtx, outCtx)

	C.av_write_trailer(outCtx)

	if (outCtx.oformat.flags & C.AVFMT_NOFILE) == 0 {
		C.avio_closep(&outCtx.pb)
	}

	return nil
}

func getAVStreamArrayElement(arrPtr **C.AVStream, i int) *C.AVStream {
	ptr := unsafe.Pointer(arrPtr)
	elemPtr := (**C.AVStream)(unsafe.Add(ptr, uintptr(i)*unsafe.Sizeof(*arrPtr)))
	return *elemPtr
}

func createAndSetupStream(streamCtx, outCtx *C.AVFormatContext) *C.AVStream {
	var outStream *C.AVStream = C.avformat_new_stream(outCtx, nil)
	C.avcodec_parameters_copy(outStream.codecpar, getAVStreamArrayElement(streamCtx.streams, 0).codecpar)
	outStream.time_base = getAVStreamArrayElement(streamCtx.streams, 0).time_base

	return outStream
}

func setupInputFile(fileName string, formatContext **C.AVFormatContext) error {
	cStr := C.CString(fileName)
	defer C.free(unsafe.Pointer(cStr))

	if C.avformat_open_input(formatContext, cStr, nil, nil) != 0 || C.avformat_find_stream_info(*formatContext, nil) < 0 {
		return fmt.Errorf("can't open [%s] file", fileName)
	}

	return nil
}

func copyStreamPackets(packet *C.AVPacket, stream *C.AVStream, streamCtx, outCtx *C.AVFormatContext) {
	for C.av_read_frame(streamCtx, packet) >= 0 {
		packet.stream_index = stream.index
		C.av_interleaved_write_frame(outCtx, packet)
		C.av_packet_unref(packet)
	}
}
