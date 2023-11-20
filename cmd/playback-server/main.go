package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"sync"
	"time"

	"github.com/danielhaba/malbeep"
	"github.com/faiface/beep"
	"github.com/faiface/beep/wav"
)

func main() {
	ctx := context.Background()
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	var httpAddress string
	var sockAddress string
	var library string
	var sampleRate uint
	var buffersLock sync.Mutex
	var buffers = make(map[string]*beep.Buffer)

	flag.StringVar(&httpAddress, "http", ":8080", "HTTP server listen address")
	flag.StringVar(&sockAddress, "socket", ":9000", "TCP server listen address")
	flag.StringVar(&library, "library", "./sounds", "Sound library location")
	flag.UintVar(&sampleRate, "sample-rate", 48000, "Sample rate")
	flag.Parse()

	audio, err := malbeep.NewSink(uint32(sampleRate))
	if err != nil {
		log.Fatalf("failed to initialize audio device: %s", err)
	}
	defer audio.Close()

	resample := func(stream beep.Streamer, format beep.Format) (beep.Streamer, beep.Format) {
		newSampleRate := beep.SampleRate(sampleRate)
		oldSampleRate := format.SampleRate
		if newSampleRate == oldSampleRate {
			return stream, format
		}
		format.SampleRate = newSampleRate

		return beep.Resample(4, oldSampleRate, newSampleRate, stream), format
	}
	load := func(fileName string) (*beep.Buffer, error) {
		buffersLock.Lock()
		defer buffersLock.Unlock()

		buffer, ok := buffers[fileName]
		if !ok {
			filePath := path.Join(library, fmt.Sprintf("%s.wav", fileName))

			file, err := os.Open(filePath)
			if err != nil {
				return nil, fmt.Errorf("failed to load file %s: %w", filePath, err)
			}
			defer file.Close()

			stream, format, err := wav.Decode(file)
			if err != nil {
				return nil, fmt.Errorf("failed to decode wav %s: %w", filePath, err)
			}
			defer stream.Close()

			resampled, format := resample(stream, format)

			buffer = beep.NewBuffer(format)
			buffer.Append(resampled)

			buffers[fileName] = buffer
		}
		return buffer, nil
	}
	play := func(reader io.Reader) {
		stream, format, err := wav.Decode(reader)
		if err != nil {
			log.Printf("failed to decode wav: %s\n", err)
			return
		}
		defer stream.Close()

		resampled, _ := resample(stream, format)

		done := make(chan struct{})
		audio.Play(beep.Seq(resampled, beep.Callback(func() {
			done <- struct{}{}
		})))
		<-done
	}
	stream := func(reader io.ReadCloser) error {
		defer reader.Close()

		data, err := io.ReadAll(reader)
		if err != nil {
			return err
		}
		play(bytes.NewReader(data))
		return nil
	}
	http.HandleFunc("/api/play", func(w http.ResponseWriter, req *http.Request) {
		var params struct {
			File string `json:"file"`
		}

		if err := json.NewDecoder(req.Body).Decode(&params); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			log.Printf("error: %s\n", err)
			return
		}
		buffer, err := load(params.File)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("error: %s\n", err)
			return
		}

		stream := buffer.Streamer(0, buffer.Len())
		done := make(chan struct{})
		audio.Play(beep.Seq(stream, beep.Callback(func() {
			done <- struct{}{}
		})))
		<-done

		w.WriteHeader(http.StatusNoContent)
	})
	httpServer := http.Server{
		Addr: httpAddress,
		BaseContext: func(l net.Listener) context.Context {
			return ctx
		},
	}

	log.Printf("tcp server started on %s\n", sockAddress)
	sockListen, err := net.Listen("tcp", sockAddress)
	if err != nil {
		log.Fatalf("failed to start tcp socket: %s\n", err)
	}

	go func() {
		<-ctx.Done()
		ctx := context.Background()
		ctx, stop := context.WithTimeout(ctx, time.Second*60)
		defer stop()
		httpServer.Shutdown(ctx)
		sockListen.Close()
	}()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for {
			conn, err := sockListen.Accept()
			if errors.Is(err, net.ErrClosed) {
				return
			}
			if err == nil {
				log.Printf("new connection from %s\n", conn.RemoteAddr())
				go func() {
					err := stream(conn)
					if err != nil {
						log.Printf("failed to read data from connection %s\n", conn.RemoteAddr())
					}
				}()
			} else {
				log.Printf("failed to accept connection: %s\n", err)
			}
		}
	}()

	go func() {
		defer wg.Done()
		log.Printf("http server started on %s\n", httpAddress)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("failed to start http server: %s\n", err)
		}
	}()

	wg.Wait()
}
