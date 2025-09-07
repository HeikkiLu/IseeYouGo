package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gocv.io/x/gocv"
)

type Device struct {
	Id         int
	Width      float64
	Height     float64
	Resolution string
	FPS        int
}

type Config struct {
	BotToken string `json:"bot_token"`
	ChatID   int64  `json:"chat_id"`
}

var devices []Device
var config Config
var bot *tgbotapi.BotAPI

func configPath() string {
	base, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	dir := filepath.Join(base, "iseeyougo")
	os.MkdirAll(dir, 0o755) // ensure directory exists
	return filepath.Join(dir, "config.json")
}

func loadConfig() {
	path := configPath()
	fmt.Println("Loading Telegram configuration from", path)

	file, err := os.Open(path)
	if err != nil {
		fmt.Println("Config not found, creating example at", path)

		example := Config{
			BotToken: "PUT_YOUR_BOT_TOKEN_HERE",
			ChatID:   0,
		}
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		exampleFile, createErr := os.Create(path)
		if createErr != nil {
			log.Printf("Cannot create %s: %v\n", path, createErr)
			return
		}
		defer exampleFile.Close()

		json.NewEncoder(exampleFile).Encode(example)
		fmt.Println("Created config.json. Edit the file and restart.")
		return
	}
	defer file.Close()

	if err := json.NewDecoder(file).Decode(&config); err != nil {
		fmt.Println("Error reading", path, ":", err)
		return
	}

	if config.BotToken == "PUT_YOUR_BOT_TOKEN_HERE" || config.ChatID == 0 {
		fmt.Println("Please edit", path, "with your bot token and chat ID")
		return
	}

	var botErr error
	bot, botErr = tgbotapi.NewBotAPI(config.BotToken)
	if botErr != nil {
		fmt.Printf("Telegram bot error: %v\n", botErr)
		return
	}
	fmt.Printf("Telegram bot connected (@%s)\n", bot.Self.UserName)
}

func sendVideo(videoPath string) {
	if bot == nil {
		fmt.Println("Telegram bot not configured, video saved locally.")
		return
	}

	fileInfo, err := os.Stat(videoPath)
	if err != nil {
		log.Printf("Cannot access video file: %v", err)
	}

	fileSizeMB := float64(fileInfo.Size()) / (1024 * 1024)
	if fileSizeMB > 50 {
		log.Printf("Video too large for Telegram (%.1f MB > 50MB", fileSizeMB)
		return
	}

	fmt.Println("Sending video to Telegram...")

	video := tgbotapi.NewVideo(config.ChatID, tgbotapi.FilePath(videoPath))
	video.Caption = fmt.Sprintf("Laptop lid opened - %s", time.Now().Format("Jan 2, 15:04:05"))

	_, botErr := bot.Send(video)

	if botErr != nil {
		log.Printf("Failed to send video to Telegram: %v", botErr)

		msg := tgbotapi.NewMessage(config.ChatID, fmt.Sprintf("Video recorded but failed to send (%.1f MB)\n%s", fileSizeMB, videoPath))

		if _, msgErr := bot.Send(msg); msgErr != nil {
			log.Printf("Failed to send notification message: %v", msgErr)
		}
		return
	}
	fmt.Println("Video sent successfully")

}

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

func monitor(dev Device, dur time.Duration) {
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
					takeVideo(dev, dur)
				}()
			}
		}
		prev = open
	}
}

// takeVideo records for 'dur' and saves timestamped MP4
func takeVideo(d Device, dur time.Duration) {
	fmt.Println("Lid opened, recording…")

	cap, err := gocv.OpenVideoCapture(d.Id)
	if err != nil || !cap.IsOpened() {
		log.Printf("open cam %d: %w", d.Id, err)
		return
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

	writer, err := gocv.VideoWriterFile(filename, "avc1", float64(fps), w, h, true)
	if err != nil {
		log.Printf("create writer: %w", err)
		return
	}

	img := gocv.NewMat()
	defer img.Close()

	deadline := time.Now().Add(dur)
	tick := time.NewTicker(time.Second / time.Duration(fps))
	defer tick.Stop()

	fmt.Printf("Recording %dx%d @ %dfps: %s\n", w, h, fps, filename)

	for range tick.C {
		if time.Now().After(deadline) {
			break
		}
		if ok := cap.Read(&img); !ok || img.Empty() {
			continue
		}
		if err := writer.Write(img); err != nil {
			fmt.Printf("Write: %w", err)
			continue
		}
	}

	writer.Close()
	time.Sleep(1 * time.Second)

	fmt.Println("Saved:", filename)
	sendVideo(filename)
}

func runCLI() {
	loadConfig()

	enumerate(3)
	if len(devices) == 0 {
		log.Fatal("No cameras found")
	}
	dev, err := chooseDevice()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Print("Length in seconds (default 15): ")

	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(line)

	dur := 15 // default
	if line != "" {
		if n, err := strconv.Atoi(line); err == nil && n > 0 {
			dur = n
		}
	}

	monitor(dev, time.Duration(dur)*time.Second)
}
