#!/bin/bash

APP_NAME="webhook-server"
APP_PATH="."
CONFIG_PATH="./config.yaml"
APP_EXEC="$APP_PATH/$APP_NAME"
PID_FILE="$APP_PATH/$APP_NAME.pid"

start_app() {
    if [ -f "$PID_FILE" ]; then
        echo "$APP_NAME is already running."
    else
        echo "Starting $APP_NAME..."
        if [ ! -f "$APP_EXEC" ]; then
            echo "Binary $APP_EXEC not found. Please run 'make build' first."
            exit 1
        fi
        if [ ! -f "$CONFIG_PATH" ]; then
            echo "Config file $CONFIG_PATH not found. Creating default config..."
            cp config.yaml.example config.yaml 2>/dev/null || echo "Warning: No example config found"
        fi
        nohup $APP_EXEC > $APP_PATH/$APP_NAME.log 2>&1 &
        echo $! > "$PID_FILE"
        echo "$APP_NAME started with PID $(cat $PID_FILE)."
        echo "Logs available at: $APP_PATH/$APP_NAME.log"
        echo "Web interface: http://localhost:8080"
        echo "API docs: http://localhost:8080/api/webhooks"
    fi
}

stop_app() {
    if [ -f "$PID_FILE" ]; then
        PID=$(cat "$PID_FILE")
        echo "Stopping $APP_NAME with PID $PID..."
        kill $PID
        sleep 2
        # Force kill if still running
        if ps -p $PID > /dev/null 2>&1; then
            echo "Force killing $APP_NAME..."
            kill -9 $PID
        fi
        rm -f "$PID_FILE"
        echo "$APP_NAME stopped."
    else
        echo "$APP_NAME is not running."
    fi
}

restart_app() {
    stop_app
    sleep 1
    start_app
}

status_app() {
    if [ -f "$PID_FILE" ]; then
        PID=$(cat "$PID_FILE")
        if ps -p $PID > /dev/null 2>&1; then
            echo "$APP_NAME is running with PID $PID."
        else
            echo "$APP_NAME PID file exists but process is not running. Cleaning up..."
            rm -f "$PID_FILE"
        fi
    else
        echo "$APP_NAME is not running."
    fi
}

logs_app() {
    if [ -f "$APP_PATH/$APP_NAME.log" ]; then
        tail -f "$APP_PATH/$APP_NAME.log"
    else
        echo "Log file not found: $APP_PATH/$APP_NAME.log"
    fi
}

case "$1" in
    start)
        start_app
        ;;
    stop)
        stop_app
        ;;
    restart)
        restart_app
        ;;
    status)
        status_app
        ;;
    logs)
        logs_app
        ;;
    *)
        echo "Usage: $0 {start|stop|restart|status|logs}"
        echo ""
        echo "Commands:"
        echo "  start   - Start the webhook server"
        echo "  stop    - Stop the webhook server"
        echo "  restart - Restart the webhook server"
        echo "  status  - Check if webhook server is running"
        echo "  logs    - Follow the application logs"
        exit 1
        ;;
esac