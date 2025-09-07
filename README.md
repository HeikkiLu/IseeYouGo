# IseeYouGo

Records video when your MacBook lid opens. Sends it to Telegram if configured. Works as both system tray application or in cli mode.

## Installation

1. **Install Go and OpenCV** (if not installed):
   ```bash
   brew install go
   brew install opencv
   brew install pkgconfig   # pkg-config is used to determine the correct flags for compiling and linking OpenCV
   ```

2. **Clone and build**:
   ```bash
   git clone https://github.com/HeikkiLu/IseeYouGo
   cd iseeyougo
   go mod tidy
   go build
   ```

3. **Run the application**:
   ```bash
   # GUI mode - compact system tray version (recommended)
   ./iseeyou

   # CLI mode
   ./iseeyou -cli

   # Show help
   ./iseeyou -help
   ```

## Telegram Setup (Optional)

1. **Create bot**: Message [@BotFather](https://t.me/BotFather): `/newbot`

2. **Get your chat ID**:
   - Send message to your bot
   - Visit: `https://api.telegram.org/botYOUR_TOKEN/getUpdates`
   - Copy the chat ID number

3. **Edit config.json**:

- The `config.json` lives in the default config directory, which on macOS is `~/Library/Application Support/iseeyougo/`

   ```json
   {
     "bot_token": "1234567890:ABC123def456...",
     "chat_id": 987654321
   }
   ```

4. **Restart the app**

## How it works

- Laptop lid is closed -> recording is 'armed'
- Laptop lid is opened -> recording starts
- Video is saved to `videos/` folder
- Video is sent to Telegram if configured

## Requirements

- **macOS only** (uses macOS-specific lid detection)
- **Camera permissions** (will be requested on first run)
- **Go 1.22+** for building from source
- **OpenCV** for camera API

## Permissions

On first run, macOS will request:
- **Camera Access**: Required for video recording
- **Accessibility Access**: May be needed for lid state detection

Grant these permissions when prompted for the app to function properly.

## Building

The app uses these main dependencies:
- `gocv.io/x/gocv` - OpenCV bindings for Go
- `fyne.io/fyne/v2` - GUI framework
- `github.com/go-telegram-bot-api/telegram-bot-api/v5` - Telegram integration
