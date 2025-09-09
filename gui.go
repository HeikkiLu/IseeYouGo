package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gocv.io/x/gocv"
)

type GUI struct {
	app           fyne.App
	window        fyne.Window
	deviceSelect  *widget.Select
	durationEntry *widget.Entry
	botTokenEntry *widget.Entry
	chatIDEntry   *widget.Entry
	statusLabel   *widget.Label
	startButton   *widget.Button
	stopButton    *widget.Button
	logText       *widget.Entry
	logContainer  *container.Scroll
	showLogButton *widget.Button
	logVisible    bool
	mainContainer *fyne.Container

	isMonitoring   bool
	stopChannel    chan bool
	selectedDevice Device
	recordDuration time.Duration
	isHidden       bool
}

func NewGUI() *GUI {
	myApp := app.NewWithID("com.github.heikkilu.iseeyougo")
	myApp.SetIcon(theme.ComputerIcon())

	w := myApp.NewWindow("IseeYouGo")
	w.Resize(fyne.NewSize(400, 320))
	w.SetFixedSize(true)
	w.CenterOnScreen()

	gui := &GUI{
		app:         myApp,
		window:      w,
		stopChannel: make(chan bool),
	}

	gui.setupUI()
	gui.setupSystemTray()
	gui.loadDevices()
	gui.loadConfiguration()

	return gui
}

func (g *GUI) setupUI() {
	// Status section
	g.statusLabel = widget.NewLabel("Ready")
	g.statusLabel.Importance = widget.MediumImportance

	// Camera selection
	g.deviceSelect = widget.NewSelect([]string{}, func(selected string) {
		g.onDeviceSelected(selected)
	})
	g.deviceSelect.PlaceHolder = "Select camera..."

	// Recording duration
	g.durationEntry = widget.NewEntry()
	g.durationEntry.SetText("15")
	g.durationEntry.SetPlaceHolder("Seconds")

	// Telegram configuration
	g.botTokenEntry = widget.NewEntry()
	g.botTokenEntry.SetPlaceHolder("Bot token")
	g.botTokenEntry.Password = true

	g.chatIDEntry = widget.NewEntry()
	g.chatIDEntry.SetPlaceHolder("Chat ID")

	// Control buttons
	g.startButton = widget.NewButton("Start", g.startMonitoring)
	g.startButton.Importance = widget.HighImportance

	g.stopButton = widget.NewButton("Stop", g.stopMonitoring)
	g.stopButton.Disable()

	// Log output, default hidden
	g.logText = widget.NewMultiLineEntry()
	g.logText.SetText("Ready to start monitoring...\n")
	g.logText.Disable()

	// Show/hide log button
	g.showLogButton = widget.NewButton("Show Log", g.toggleLog)
	g.logVisible = false

	// Compact layout with tooltips
	testButton := widget.NewButton("Test", g.testTelegramConnection)
	testButton.Resize(fyne.NewSize(50, testButton.MinSize().Height))

	cameraRow := container.NewGridWithColumns(2,
		g.deviceSelect, g.durationEntry,
	)

	telegramRow := container.NewGridWithColumns(3,
		g.botTokenEntry, g.chatIDEntry, testButton,
	)

	controlsRow := container.NewGridWithColumns(3,
		g.startButton, g.stopButton, g.showLogButton,
	)

	g.logContainer = container.NewScroll(g.logText)
	g.logContainer.SetMinSize(fyne.NewSize(380, 80))

	// Create compact labels
	cameraLabel := widget.NewLabel("Camera & Duration:")
	cameraLabel.TextStyle.Bold = true
	telegramLabel := widget.NewLabel("Telegram (optional):")
	telegramLabel.TextStyle.Bold = true

	g.mainContainer = container.NewVBox(
		container.NewHBox(
			widget.NewIcon(theme.InfoIcon()),
			g.statusLabel,
		),
		widget.NewSeparator(),
		cameraLabel,
		cameraRow,
		telegramLabel,
		telegramRow,
		widget.NewSeparator(),
		controlsRow,
	)

	mainContainer := g.mainContainer

	g.window.SetContent(mainContainer)

	// Set close intercept to hide instead of quit
	g.window.SetCloseIntercept(func() {
		g.hideToSystemTray()
	})

	// Add helpful tooltips
	g.addTooltips()
}

func (g *GUI) loadDevices() {
	g.appendLog("Scanning for cameras...")

	// Scan for devices in background, but update UI on main thread
	devices = []Device{} // Reset global devices
	enumerate(10)        // Scan more devices for GUI

	if len(devices) == 0 {
		g.appendLog("No cameras found!")
		return
	}

	options := make([]string, len(devices))
	for i, d := range devices {
		options[i] = fmt.Sprintf("Camera %d - %s @ %dfps", d.Id, d.Resolution, d.FPS)
	}

	g.deviceSelect.Options = options
	if len(options) > 0 {
		g.deviceSelect.SetSelected(options[0])
	}

	g.appendLog(fmt.Sprintf("Found %d camera(s)", len(devices)))
}

func (g *GUI) loadConfiguration() {
	path := configPath()
	file, err := os.Open(path)
	if err != nil {
		g.appendLog("No existing configuration found")
		return
	}
	defer file.Close()

	var cfg Config
	if err := json.NewDecoder(file).Decode(&cfg); err != nil {
		g.appendLog("Error reading configuration: " + err.Error())
		return
	}

	if cfg.BotToken != "PUT_YOUR_BOT_TOKEN_HERE" && cfg.BotToken != "" {
		g.botTokenEntry.SetText(cfg.BotToken)
	}

	if cfg.ChatID != 0 {
		g.chatIDEntry.SetText(strconv.FormatInt(cfg.ChatID, 10))
	}

	g.appendLog("Configuration loaded")
}

func (g *GUI) onDeviceSelected(selected string) {
	if selected == "" {
		return
	}

	// Parse device index from selection
	for _, d := range devices {
		option := fmt.Sprintf("Camera %d - %s @ %dfps", d.Id, d.Resolution, d.FPS)
		if option == selected {
			g.selectedDevice = d
			g.appendLog(fmt.Sprintf("Selected: %s", selected))
			return
		}
	}
}

func (g *GUI) startMonitoring() {
	// Validate inputs
	if g.deviceSelect.Selected == "" {
		dialog.ShowError(fmt.Errorf("please select a camera"), g.window)
		return
	}

	duration, err := strconv.Atoi(g.durationEntry.Text)
	if err != nil || duration <= 0 {
		dialog.ShowError(fmt.Errorf("please enter a valid duration in seconds"), g.window)
		return
	}
	g.recordDuration = time.Duration(duration) * time.Second

	// Save configuration
	g.saveConfiguration()

	// Setup Telegram if configured
	if g.botTokenEntry.Text != "" && g.chatIDEntry.Text != "" {
		g.setupTelegram()
	}

	// Start monitoring
	g.isMonitoring = true
	g.startButton.Disable()
	g.botTokenEntry.Disable()
	g.chatIDEntry.Disable()
	g.stopButton.Enable()
	g.statusLabel.SetText("Monitoring - waiting for lid close/open")
	g.appendLog("Started monitoring lid state...")

	go g.monitorLidState()
}

func (g *GUI) stopMonitoring() {
	if g.isMonitoring {
		g.stopChannel <- true
		g.isMonitoring = false
	}

	g.startButton.Enable()
	g.stopButton.Disable()
	g.statusLabel.SetText("Stopped")
	g.appendLog("Monitoring stopped")
}

func (g *GUI) monitorLidState() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	armed := false
	var prev bool
	havePrev := false
	lastTrigger := time.Time{}
	const cooldown = 5 * time.Second

	for {
		select {
		case <-g.stopChannel:
			return
		case <-ticker.C:
			open, err := checkLidStatus()
			if err != nil {
				continue
			}

			if !havePrev {
				prev = open
				havePrev = true
				if !open {
					g.statusLabel.SetText("Monitoring - lid closed (armed)")
				} else {
					g.statusLabel.SetText("Monitoring - lid open")
				}
			}

			if !open && prev {
				armed = true
				g.statusLabel.SetText("Monitoring - lid closed (armed)")
				g.appendLog("Lid closed - recording armed")
			}

			if armed && !prev && open {
				if time.Since(lastTrigger) > cooldown {
					lastTrigger = time.Now()
					armed = false
					g.statusLabel.SetText("Recording...")
					g.appendLog("Lid opened - starting recording!")
					go g.recordVideo()
				} else {
					g.appendLog("Lid opened but still in cooldown period")
				}
			}

			prev = open
		}
	}
}

func (g *GUI) recordVideo() {
	g.appendLog("Starting video recording...")

	cap, err := gocv.OpenVideoCapture(g.selectedDevice.Id)
	if err != nil || !cap.IsOpened() {
		g.appendLog(fmt.Sprintf("Error opening camera %d: %v", g.selectedDevice.Id, err))
		g.statusLabel.SetText("Error - Camera unavailable")
		return
	}
	defer cap.Close()

	w, h := int(g.selectedDevice.Width), int(g.selectedDevice.Height)
	if w == 0 || h == 0 {
		w, h = 1280, 720
	}

	cap.Set(gocv.VideoCaptureFrameWidth, float64(w))
	cap.Set(gocv.VideoCaptureFrameHeight, float64(h))

	fps := g.selectedDevice.FPS
	if fps <= 0 {
		fps = 30
	}

	home, err := os.UserHomeDir()
	if err != nil {
		g.appendLog(fmt.Sprintf("Error finding home dir: %v", err))
		return
	}

	ts := time.Now().Format("20060102_150405")
	dir := filepath.Join(home, "iseeyougo", "videos")
	_ = os.MkdirAll(dir, 0o755)

	filename := filepath.Join(dir, fmt.Sprintf("capture_%s.mp4", ts))

	writer, err := gocv.VideoWriterFile(filename, "avc1", float64(fps), w, h, true)
	if err != nil {
		g.appendLog(fmt.Sprintf("Error creating video writer: %v", err))
		g.statusLabel.SetText("Error - Cannot create video file")
		return
	}

	img := gocv.NewMat()
	defer img.Close()

	deadline := time.Now().Add(g.recordDuration)
	tick := time.NewTicker(time.Second / time.Duration(fps))
	defer tick.Stop()

	g.appendLog(fmt.Sprintf("Recording %dx%d @ %dfps for %v seconds...", w, h, fps, g.recordDuration.Seconds()))

	frameCount := 0
	for range tick.C {
		if time.Now().After(deadline) {
			break
		}

		if ok := cap.Read(&img); !ok || img.Empty() {
			continue
		}

		if err := writer.Write(img); err != nil {
			g.appendLog(fmt.Sprintf("Error writing frame: %v", err))
			continue
		}
		frameCount++
	}

	writer.Close()
	time.Sleep(1 * time.Second)

	g.appendLog(fmt.Sprintf("Recording complete! Saved %d frames to: %s", frameCount, filename))
	g.statusLabel.SetText("Monitoring - recording complete")

	// Send to Telegram if configured
	if bot != nil {
		g.sendVideoToTelegram(filename)
	}
}

func (g *GUI) sendVideoToTelegram(videoPath string) {
	g.appendLog("Sending video to Telegram...")

	fileInfo, err := os.Stat(videoPath)
	if err != nil {
		g.appendLog(fmt.Sprintf("Cannot access video file: %v", err))
		return
	}

	fileSizeMB := float64(fileInfo.Size()) / (1024 * 1024)
	if fileSizeMB > 50 {
		g.appendLog(fmt.Sprintf("Video too large for Telegram (%.1f MB > 50MB)", fileSizeMB))
		return
	}

	video := tgbotapi.NewVideo(config.ChatID, tgbotapi.FilePath(videoPath))
	video.Caption = fmt.Sprintf("Laptop lid opened - %s", time.Now().Format("Jan 2, 15:04:05"))

	_, botErr := bot.Send(video)
	if botErr != nil {
		g.appendLog(fmt.Sprintf("Failed to send video to Telegram: %v", botErr))
		return
	}

	g.appendLog("Video sent to Telegram successfully!")
}

func (g *GUI) testTelegramConnection() {
	if g.botTokenEntry.Text == "" {
		dialog.ShowError(fmt.Errorf("please enter bot token"), g.window)
		return
	}

	g.appendLog("Testing Telegram connection...")

	testBot, err := tgbotapi.NewBotAPI(g.botTokenEntry.Text)
	if err != nil {
		g.appendLog(fmt.Sprintf("Telegram connection failed: %v", err))
		dialog.ShowError(fmt.Errorf("invalid bot token: %v", err), g.window)
		return
	}

	g.appendLog(fmt.Sprintf("Successfully connected to Telegram bot: @%s", testBot.Self.UserName))

	if g.chatIDEntry.Text != "" {
		chatID, err := strconv.ParseInt(g.chatIDEntry.Text, 10, 64)
		if err == nil {
			msg := tgbotapi.NewMessage(chatID, "IseeYouGo test message - connection successful!")
			if _, err := testBot.Send(msg); err != nil {
				g.appendLog(fmt.Sprintf("Failed to send test message: %v", err))
			} else {
				g.appendLog("Test message sent successfully!")
				dialog.ShowInformation("Success", "Telegram connection test successful!", g.window)
			}
		}
	} else {
		dialog.ShowInformation("Success", fmt.Sprintf("Bot connection successful (@%s)!\nAdd chat ID to send test messages.", testBot.Self.UserName), g.window)
	}
}

func (g *GUI) saveConfiguration() {
	path := configPath()

	cfg := Config{}

	if g.botTokenEntry.Text != "" {
		cfg.BotToken = g.botTokenEntry.Text
	} else {
		cfg.BotToken = "PUT_YOUR_BOT_TOKEN_HERE"
	}

	if g.chatIDEntry.Text != "" {
		if chatID, err := strconv.ParseInt(g.chatIDEntry.Text, 10, 64); err == nil {
			cfg.ChatID = chatID
		}
	}

	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	file, err := os.Create(path)
	if err != nil {
		g.appendLog(fmt.Sprintf("Cannot save configuration: %v", err))
		return
	}
	defer file.Close()

	if err := json.NewEncoder(file).Encode(cfg); err != nil {
		g.appendLog(fmt.Sprintf("Error saving configuration: %v", err))
		return
	}

	config = cfg
	g.appendLog("Configuration saved")
}

func (g *GUI) setupTelegram() {
	if config.BotToken == "PUT_YOUR_BOT_TOKEN_HERE" || config.ChatID == 0 {
		return
	}

	var err error
	bot, err = tgbotapi.NewBotAPI(config.BotToken)
	if err != nil {
		g.appendLog(fmt.Sprintf("Telegram bot error: %v", err))
		return
	}

	g.appendLog(fmt.Sprintf("Telegram bot connected (@%s)", bot.Self.UserName))
}

func (g *GUI) appendLog(message string) {
	timestamp := time.Now().Format("15:04:05")
	logEntry := fmt.Sprintf("[%s] %s\n", timestamp, message)

	// Update UI from main thread
	currentText := g.logText.Text
	g.logText.SetText(currentText + logEntry)

	// Auto-scroll to bottom (simulate by setting cursor to end)
	g.logText.CursorRow = len(strings.Split(g.logText.Text, "\n"))
}

func (g *GUI) setupSystemTray() {
	if desk, ok := g.app.(desktop.App); ok {
		menu := fyne.NewMenu("IseeYouGo",
			fyne.NewMenuItem("Show", func() {
				g.showFromSystemTray()
			}),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Start Monitoring", func() {
				if !g.isMonitoring {
					g.startMonitoring()
				}
			}),
			fyne.NewMenuItem("Stop Monitoring", func() {
				if g.isMonitoring {
					g.stopMonitoring()
				}
			}),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Quit", func() {
				g.quitApplication()
			}),
		)
		desk.SetSystemTrayMenu(menu)
		desk.SetSystemTrayIcon(theme.ComputerIcon())
	}
}

func (g *GUI) hideToSystemTray() {
	g.window.Hide()
	g.isHidden = true
	g.appendLog("Application minimized to system tray")

}

func (g *GUI) showFromSystemTray() {
	g.window.Show()
	g.isHidden = false
	g.appendLog("Application restored from system tray")
}

func (g *GUI) quitApplication() {
	if g.isMonitoring {
		g.stopMonitoring()
	}
	g.app.Quit()
}

func (g *GUI) Run() {
	g.window.ShowAndRun()
}

func (g *GUI) toggleLog() {
	if g.logVisible {
		// Hide log
		objects := g.mainContainer.Objects
		if len(objects) >= 2 {
			// Remove separator and log container (last two items)
			g.mainContainer.Objects = objects[:len(objects)-2]
		}
		g.showLogButton.SetText("Show Log")
		g.window.Resize(fyne.NewSize(400, 220))
		g.logVisible = false
		g.mainContainer.Refresh()
	} else {
		// Show log
		g.mainContainer.Add(widget.NewSeparator())
		g.mainContainer.Add(g.logContainer)
		g.showLogButton.SetText("Hide Log")
		g.window.Resize(fyne.NewSize(400, 320))
		g.logVisible = true
		g.mainContainer.Refresh()
	}
}

func (g *GUI) addTooltips() {
	// Add helpful placeholder text for compact mode
	g.deviceSelect.PlaceHolder = "Select camera device..."
}

func (g *GUI) Close() {
	g.quitApplication()
}
