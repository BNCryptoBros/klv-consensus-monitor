# Klever Validator Consensus Monitor

A lightweight Go program that monitors Klever blockchain validators and sends notifications when their status changes across epochs.

## Features

- Monitors validator status changes (elected, eligible, waiting, jailed)
- Tracks epoch transitions
- Slack webhook notifications with customizable messages
- Telegram bot notifications with customizable messages
- Clean, human-readable logs
- Runs continuously in the background

## Configuration

Edit `config.yaml`:

```yaml
nodeBaseUrl: "https://node.mainnet.klever.org"
apiBaseUrl: "https://api.mainnet.klever.org"
pollInterval: 30

validators:
  - blsKey: "your_validator_bls_public_key_here"
    displayName: "My Validator"

slack:
  enabled: true
  webhookUrl: "https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
  messageTemplate: |
    {
      "text": "{{displayName}} status: {{oldStatus}} → {{newStatus}} (Epoch {{epoch}})"
    }

telegram:
  enabled: true
  botToken: "123456:ABC-your-bot-token"
  chatId: "-1001234567890"
  messageTemplate: |
    <b>{{displayName}}</b> status: {{oldStatus}} → {{newStatus}} (Epoch {{epoch}})
```

Slack and Telegram are independent: each one has its own `enabled` flag and can be turned on or off without affecting the other. This applies to both validator status changes (`monitor` mode) and the payday summary (`payments` mode).

**Required fields:**
- `validators[].blsKey`: 192-character BLS public key
- `validators[].displayName`: Human-readable name

**Telegram setup:**
- Create a bot with [@BotFather](https://t.me/BotFather) and copy its token into `telegram.botToken`
- Add the bot to the target group/channel and put its chat ID in `telegram.chatId`
- `telegram.messageTemplate` uses Telegram HTML formatting (`<b>`, `<i>`, `<code>`, `<a href>`); if omitted, a default template is used

**Message placeholders (Slack and Telegram):**
- `{{displayName}}`: Validator name
- `{{oldStatus}}`: Previous status
- `{{newStatus}}`: Current status
- `{{epoch}}`: Epoch number

## Running

**Build and run:**
```bash
go build -o klv-monitor
./klv-monitor
```

**Or run directly:**
```bash
go run main.go
```

**Stop:** Press `Ctrl+C`

## Output Example

```
Loaded configuration: monitoring 2 validators
Initial epoch: 4914
[2025-01-11 10:30:00] BN 1 - Initial status: elected (Epoch: 4914)
[2025-01-11 10:35:00] New epoch detected: 4915 (previous: 4914)
[2025-01-11 10:35:00] BN 1 - Status changed: elected → waiting (Epoch: 4915)
Slack notification sent for BN 1 status change
```

## Requirements

- Go 1.25+
- Internet connection to Klever APIs
- Valid BLS public keys for validators to monitor
