package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

    "github.com/danielhaba/audio-server/streamer"
	"github.com/gopxl/beep"
	"github.com/gopxl/beep/speaker"
	"github.com/gopxl/beep/wav"
	"github.com/gin-gonic/gin"
)

var (
	Library  streamer.Library
	Channels streamer.Mixer
	Config   struct {
		Address     string
		LibraryPath string
		SampleRate  uint
	}
)

func Resample(stream beep.Streamer, format beep.Format) (beep.Streamer, beep.Format) {
	newSampleRate := beep.SampleRate(Config.SampleRate)
	oldSampleRate := format.SampleRate
	if newSampleRate == oldSampleRate {
		return stream, format
	}
	format.SampleRate = newSampleRate

	return beep.Resample(4, oldSampleRate, newSampleRate, stream), format
}

func ReadBuffer(reader io.Reader) (*beep.Buffer, error) {
	stream, format, err := wav.Decode(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to decode wav: %w", err)
	}
	resampled, format := Resample(stream, format)
	buffer := beep.NewBuffer(format)
	buffer.Append(resampled)

	return buffer, nil
}

func ReadStream(reader io.Reader) (beep.Streamer, error) {
	buffer, err := ReadBuffer(reader)
	if err != nil {
		return nil, err
	}
	return buffer.Streamer(0, buffer.Len()), nil
}

func LoadSample(fileName string) (beep.Streamer, error) {
	var err error
	buffer := Library.Insert(fileName, func() *beep.Buffer {
		filePath := path.Join(Config.LibraryPath, fmt.Sprintf("%s.wav", fileName))

		file, e := os.Open(filePath)
		if e != nil {
			err = fmt.Errorf("failed to open file %s: %w", filePath, e)
			return nil
		}
		defer file.Close()

		buffer, e := ReadBuffer(file)
		if e != nil {
			err = fmt.Errorf("failed to decode file %s: %w", filePath, e)
		}
		return buffer
	})
	if err != nil {
		return nil, err
	}
	return buffer.Streamer(0, buffer.Len()), nil
}

func Play(stream beep.Streamer, channel *string) {
	if channel != nil {
		Channels.Insert(*channel, func() beep.Streamer {
			return &streamer.Channel{}
		}).(*streamer.Channel).Add(stream)
	} else {
        speaker.Play(stream)
	}
}

func main() {
	flag.StringVar(&Config.Address, "addr", ":8000", "HTTP server listen address")
	flag.StringVar(&Config.LibraryPath, "library-path", "./sounds", "Sound library location")
	flag.UintVar(&Config.SampleRate, "sample-rate", 48000, "Sample rate")
	flag.Parse()

    sampleRate := beep.SampleRate(Config.SampleRate)
    if err := speaker.Init(sampleRate, sampleRate.N(time.Second/10)); err != nil {
        log.Fatalf("Failed to initialize audio device: %s", err) 
    }
    defer speaker.Close()
    speaker.Play(&Channels)

	r := gin.Default()
	r.POST("/api/play/:channel", func(ctx *gin.Context) {
		channel := strings.Trim(ctx.Param("channel"), "/")
		stream, err := ReadStream(ctx.Request.Body)
		if err != nil {
			ctx.AbortWithError(http.StatusInternalServerError, err)
			return
		}
		Play(stream, &channel)
	})
	r.POST("/api/play", func(ctx *gin.Context) {
		stream, err := ReadStream(ctx.Request.Body)
		if err != nil {
			ctx.AbortWithError(http.StatusInternalServerError, err)
			return
		}
		Play(stream, nil)
	})
	r.POST("/api/play-sample/:sample", func(ctx *gin.Context) {
		var (
			params = strings.Split(strings.Trim(ctx.Param("sample"), "/"), "/")
			sample = params[0]
            channel  *string
		)
		if len(params) > 1 {
            channel = &params[0]
            sample = params[1]
		}

		stream, err := LoadSample(sample)
		if err != nil {
			ctx.AbortWithError(http.StatusInternalServerError, err)
			return
		}
		Play(stream, channel)
	})
	r.Run(Config.Address)
}
