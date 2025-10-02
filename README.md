# Webhook Testing Tool

A powerful Go application that simulates webhook.site functionality - measures TPS, logs requests, and provides configurable responses. Perfect for testing webhooks, API endpoints, and measuring system performance.

## ✨ Features

- **🎯 Configurable Webhook Endpoint**: Set custom status codes, content types, response bodies, and timeouts
- **📊 Real-time TPS Metrics**: Monitor transactions per second and performance metrics
- **📝 Request Logging**: Automatically log all incoming requests to JSON file
- **🌐 Web Interface**: Beautiful web UI for configuration and monitoring
- **⚡ High Performance**: Thread-safe implementation with mutex locks
- **🔄 Auto-refresh**: Live updates of metrics and request logs

## 🚀 Quick Start

1. **Clone and run:**
```bash
git clone <repository-url>
cd measure-tps-by-receive-request
go mod tidy
go run main.go
```

2. **Access the web interface:**
   - Open http://localhost:8080 in your browser
   - Your webhook URL: http://localhost:8080/webhook

3. **Start testing:**
   - Send requests to `/webhook` endpoint
   - Configure responses via web interface
   - Monitor real-time metrics and logs

## 🎛️ Web Interface

The web interface provides:

- **Configuration Panel**: Set status codes, content types, response delays, and custom headers
- **Live Metrics**: View TPS, total requests, duration, and status
- **Request Logs**: See all incoming requests with details
- **One-click Actions**: Reset metrics, clear logs, copy webhook URL

## 📡 API Endpoints

### Main Webhook Endpoint
- **`ANY /webhook`** - Configurable webhook endpoint that logs requests and returns custom responses

### Configuration
- **`GET /api/config`** - Get current webhook configuration
- **`POST /api/config`** - Update webhook configuration

### Request Logs
- **`GET /api/requests`** - Get all logged requests
- **`DELETE /api/requests`** - Clear request logs

### Metrics
- **`GET /api/metrics`** - Get TPS metrics
- **`POST /api/reset`** - Reset metrics

### Legacy Endpoints (backward compatibility)
- **`POST /api/request`** - Record a request for TPS calculation
- **`GET /api/metrics`** - Get current metrics
- **`POST /api/reset`** - Reset metrics

## ⚙️ Configuration Options

Configure your webhook via the web interface or API:

```json
{
  "status_code": 200,
  "content_type": "application/json",
  "response_body": "{\"message\": \"Request received\"}",
  "timeout": 1000,
  "headers": {
    "X-Custom-Header": "value",
    "X-Powered-By": "Webhook-Tool"
  }
}
```

## 📋 Usage Examples

### Basic Webhook Testing
```bash
# Send a simple POST request
curl -X POST http://localhost:8080/webhook \
  -H "Content-Type: application/json" \
  -d '{"test": "data"}'

# Send with custom headers
curl -X POST http://localhost:8080/webhook \
  -H "User-Agent: MyApp/1.0" \
  -H "X-API-Key: secret123" \
  -d "test payload"
```

### Configure Custom Response
```bash
# Set custom status code and response
curl -X POST http://localhost:8080/api/config \
  -H "Content-Type: application/json" \
  -d '{
    "status_code": 201,
    "content_type": "application/json",
    "response_body": "{\"status\": \"created\", \"id\": 123}",
    "timeout": 500,
    "headers": {"X-Request-ID": "abc123"}
  }'
```

### Monitor Performance
```bash
# Get real-time metrics
curl http://localhost:8080/api/metrics

# View request logs
curl http://localhost:8080/api/requests
```

## 📊 Metrics

The tool tracks:
- **Total Requests**: Number of requests received
- **TPS (Transactions Per Second)**: Real-time throughput
- **Duration**: Time since first request
- **Status**: Active/Waiting indicator

## 📁 File Structure

```
├── main.go           # Main application
├── static/
│   └── index.html    # Web interface
├── requests.json     # Request logs (auto-created)
├── go.mod           # Go dependencies
└── README.md        # This file
```

## 🛠️ Technical Details

- **Go Version**: 1.22.6
- **Framework**: Gin web framework
- **Concurrency**: Thread-safe with `sync.RWMutex`
- **Storage**: JSON file for request logs (last 1000 requests)
- **Performance**: Optimized for high-throughput testing

## 📦 Dependencies

- [Gin](https://github.com/gin-gonic/gin) - HTTP web framework

## 🔧 Advanced Usage

### Load Testing
Use this tool to test your applications' webhook handling:

```bash
# Generate load with curl
for i in {1..100}; do
  curl -X POST http://localhost:8080/webhook \
    -d "test-$i" &
done
```

### Response Simulation
Simulate different server responses for testing error handling:

- Set status code to 500 to test error handling
- Add delays to test timeout handling
- Return custom JSON to test response parsing

### Integration Testing
Perfect for testing webhook integrations with services like:
- GitHub webhooks
- Stripe payment notifications  
- Discord/Slack bot webhooks
- Custom API callbacks