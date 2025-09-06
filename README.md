# IseeYouGo

Records video when your MacBook lid opens. Sends it to Telegram if configured.

## Installation

1. **Install Go** (if not installed):
   ```bash
   brew install go
   ```

2. **Clone and run**:
   ```bash
   git clone https://github.com/HeikkiLu/IseeYouGo
   cd iseeyougo
   go mod tidy
   go run main.go
   ```

3. **Choose camera** and start monitoring

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

- macOS only
- Camera permissions
- Go 1.22+
