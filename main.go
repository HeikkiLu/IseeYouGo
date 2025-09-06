package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gocv.io/x/gocv"
)

type Device struct {
	Id         int
	Width      float64
	Height     float64
	Resolution string
	FPS        int
}

var devices []Device

// true=open, false=closed
func checkLidStatus() (bool, error) {
	cmd := exec.Command("ioreg", "-r", "-k", "AppleClamshellState", "-d", "1")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("failed to run ioreg: %w", err)
	}
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.Contains(line, "AppleClamshellState") {
			if strings.Contains(line, "= Yes") {
				return false, nil
			}
			if strings.Contains(line, "= No") {
				return true, nil
			}
		}
	}
	return false, fmt.Errorf("AppleClamshellState not found")
}

func enumerate(max int) {
	for i := 0; i < max; i++ {
		cap, err := gocv.OpenVideoCapture(i)
		if err != nil || !cap.IsOpened() {
			continue
		}
		w := cap.Get(gocv.VideoCaptureFrameWidth)
		h := cap.Get(gocv.VideoCaptureFrameHeight)
		fps := int(cap.Get(gocv.VideoCaptureFPS))
		if fps <= 0 {
			fps = 30
		}
		devices = append(devices, Device{
			Id:         i,
			Width:      w,
			Height:     h,
			Resolution: fmt.Sprintf("%.0fx%.0f", w, h),
			FPS:        fps,
		})
		cap.Close()
	}
}

func chooseDevice() (Device, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Available cameras:")
	for idx, d := range devices {
		fmt.Printf("  [%d] id=%d  %s @ %dfps\n", idx, d.Id, d.Resolution, d.FPS)
	}
	fmt.Print("Pick camera: ")
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	n, err := strconv.Atoi(line)
	if err != nil || n < 0 || n >= len(devices) {
		return Device{}, fmt.Errorf("invalid choice")
	}
	return devices[n], nil
}

func monitor(d Device) {
	log.Println("Monitoring lid state... (Ctrl+C to quit)")
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	armed := false
	var prev bool
	havePrev := false
	lastTrigger := time.Time{}
	const cooldown = 5 * time.Second

	for range ticker.C {
		open, err := checkLidStatus()
		if err != nil {
			continue
		}
		if !havePrev {
			prev = open
			havePrev = true
		}
		if !open {
			armed = true
		}
		if armed && !prev && open {
			if time.Since(lastTrigger) > cooldown {
				lastTrigger = time.Now()
				armed = false
				go func() {
					if err := takeVideo(d, 10*time.Second); err != nil {
						log.Printf("takeVideo: %v", err)
					}
				}()
			}
		}
		prev = open
	}
}

// takeVideo records for 'dur' and saves timestamped MP4
func takeVideo(d Device, dur time.Duration) error {
	fmt.Println("Lid opened → recording…")

	cap, err := gocv.OpenVideoCapture(d.Id)
	if err != nil || !cap.IsOpened() {
		return fmt.Errorf("open cam %d: %w", d.Id, err)
	}
	defer cap.Close()

	w, h := int(d.Width), int(d.Height)
	if w == 0 || h == 0 {
		w, h = 1280, 720
	}
	cap.Set(gocv.VideoCaptureFrameWidth, float64(w))
	cap.Set(gocv.VideoCaptureFrameHeight, float64(h))
	fps := d.FPS
	if fps <= 0 {
		fps = 30
	}

	ts := time.Now().Format("20060102_150405")
	dir := "videos"
	_ = os.MkdirAll(dir, 0o755)
	filename := filepath.Join(dir, fmt.Sprintf("capture_%s.mp4", ts))

	writer, err := gocv.VideoWriterFile(filename, "mp4v", float64(fps), w, h, true)
	if err != nil {
		return fmt.Errorf("create writer: %w", err)
	}
	defer writer.Close()

	img := gocv.NewMat()
	defer img.Close()

	deadline := time.Now().Add(dur)
	tick := time.NewTicker(time.Second / time.Duration(fps))
	defer tick.Stop()

	fmt.Printf("Recording %dx%d @ %dfps → %s\n", w, h, fps, filename)
	for range tick.C {
		if time.Now().After(deadline) {
			break
		}
		if ok := cap.Read(&img); !ok || img.Empty() {
			continue
		}
		if err := writer.Write(img); err != nil {
			return fmt.Errorf("write: %w", err)
		}
	}
	fmt.Println("Saved:", filename)
	return nil
}

func main() {
	enumerate(8)
	if len(devices) == 0 {
		log.Fatal("no cameras found")
	}
	d, err := chooseDevice()
	if err != nil {
		log.Fatal(err)
	}

	// Optional: ask duration
	fmt.Print("Length in seconds (default 15): ")
	var s int
	if _, scanErr := fmt.Scan(&s); scanErr != nil || s <= 0 {
		s = 15
	}
	// Pass duration into monitor via closure if you like; here we fixed 10s in monitor.
	_ = s

	monitor(d)
}
