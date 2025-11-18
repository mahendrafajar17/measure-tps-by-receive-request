#!/bin/bash

# Load configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_FILE="$SCRIPT_DIR/monitor.config"

if [ -f "$CONFIG_FILE" ]; then
    source "$CONFIG_FILE"
else
    # Default values if config file doesn't exist
    TARGET_APP="reqforwarder"
    PROCESS_PATTERNS="main.go reqforwarder"
    LOG_FILE="reqforwarder_peaks.log"
    PID_FILE="monitor.pid"
    INTERVAL_SECONDS=1
    ENABLED=true
fi

show_help() {
    echo "Usage: ./monitor {start|stop|status|view|help}"
    echo ""
    echo "Commands:"
    echo "  start   - Start monitoring $TARGET_APP in background"
    echo "  stop    - Stop monitoring"
    echo "  status  - Show monitoring status"
    echo "  view    - View peak monitoring results"
    echo "  help    - Show this help message"
    echo ""
    echo "Configuration file: $CONFIG_FILE"
    echo "Target app: $TARGET_APP"
}

start_monitoring() {
    if [ "$ENABLED" != "true" ]; then
        echo "❌ Monitoring is disabled in configuration"
        return 1
    fi

    if [ -f "$PID_FILE" ] && kill -0 $(cat $PID_FILE 2>/dev/null) 2>/dev/null; then
        echo "❌ Monitoring already running (PID: $(cat $PID_FILE))"
        return 1
    fi

    echo "Starting $TARGET_APP monitoring..."
    echo "Timestamp,CPU%,Memory%,MemoryMB" > $LOG_FILE
    
    (
        while true; do
            STATS=$(ps aux | grep -E "$PROCESS_PATTERNS" | grep -v grep | head -1)
            if [ ! -z "$STATS" ]; then
                CPU=$(echo $STATS | awk '{print $3}')
                MEM=$(echo $STATS | awk '{print $4}')
                MEM_MB=$(echo $STATS | awk '{print $6/1024}')
                echo "$(date '+%Y-%m-%d %H:%M:%S'),$CPU,$MEM,$MEM_MB" >> $LOG_FILE
            fi
            sleep $INTERVAL_SECONDS
        done
    ) &
    
    echo $! > $PID_FILE
    echo "✅ Monitoring started (PID: $(cat $PID_FILE))"
    echo "📊 Log file: $LOG_FILE"
    echo "🎯 Target app: $TARGET_APP"
    echo "⏱️  Interval: ${INTERVAL_SECONDS}s"
}

stop_monitoring() {
    if [ ! -f "$PID_FILE" ]; then
        echo "❌ Monitor PID file not found"
        return 1
    fi
    
    PID=$(cat $PID_FILE)
    if kill -0 $PID 2>/dev/null; then
        kill $PID
        rm -f $PID_FILE
        echo "✅ Monitoring stopped (PID: $PID)"
    else
        echo "❌ Monitor process not running"
        rm -f $PID_FILE
    fi
}

show_status() {
    echo "=== MONITORING STATUS ==="
    echo "🎯 Target app: $TARGET_APP"
    echo "📄 Config file: $CONFIG_FILE"
    echo "📊 Enabled: $ENABLED"
    echo ""
    
    # Check target app process
    if pgrep -f "$PROCESS_PATTERNS" > /dev/null; then
        echo "✅ $TARGET_APP: Running"
        ps aux | grep -E "$PROCESS_PATTERNS" | grep -v grep | head -1 | awk '{print "   CPU: " $3 "%, Memory: " $4 "%, Memory MB: " $6/1024}'
    else
        echo "❌ $TARGET_APP: Not running"
    fi
    
    # Check monitor process
    if [ -f "$PID_FILE" ] && kill -0 $(cat $PID_FILE 2>/dev/null) 2>/dev/null; then
        echo "✅ Monitor: Running (PID: $(cat $PID_FILE))"
    else
        echo "❌ Monitor: Not running"
    fi
    
    # Check log file
    if [ -f "$LOG_FILE" ]; then
        RECORDS=$(tail -n +2 "$LOG_FILE" | wc -l)
        echo "📊 Log records: $RECORDS"
        if [ $RECORDS -gt 0 ]; then
            echo "   Latest entry: $(tail -n 1 $LOG_FILE)"
        fi
    else
        echo "📊 Log file: Not found"
    fi
}

view_results() {
    if [ ! -f "$LOG_FILE" ]; then
        echo "❌ Log file not found: $LOG_FILE"
        echo "Run './monitor start' first to begin monitoring"
        return 1
    fi

    echo "=== $TARGET_APP PEAK MONITORING RESULTS ==="
    echo ""

    # Total records
    TOTAL_RECORDS=$(tail -n +2 "$LOG_FILE" | wc -l)
    echo "📊 Total monitoring records: $TOTAL_RECORDS"

    if [ $TOTAL_RECORDS -eq 0 ]; then
        echo "No monitoring data available yet."
        return 0
    fi

    echo ""

    # Peak CPU
    echo "=== 🔥 TOP 5 PEAK CPU USAGE ==="
    echo "Timestamp,CPU%,Memory%,MemoryMB"
    sort -t, -k2 -nr "$LOG_FILE" | tail -n +2 | head -5

    echo ""

    # Peak Memory
    echo "=== 🧠 TOP 5 PEAK MEMORY USAGE ==="
    echo "Timestamp,CPU%,Memory%,MemoryMB"
    sort -t, -k3 -nr "$LOG_FILE" | tail -n +2 | head -5

    echo ""

    # Summary
    echo "=== 📈 SUMMARY ==="
    awk -F, 'NR>1 {
        if($2>maxcpu) {maxcpu=$2; maxcpu_time=$1}
        if($3>maxmem) {maxmem=$3; maxmem_time=$1}
        if($4>maxmem_mb) {maxmem_mb=$4; maxmem_mb_time=$1}
        totalcpu+=$2; totalmem+=$3; count++
    } END {
        print "🔥 Peak CPU: " maxcpu "% at " maxcpu_time
        print "🧠 Peak Memory: " maxmem "% at " maxmem_time  
        print "💾 Peak Memory MB: " maxmem_mb " MB at " maxmem_mb_time
        print "📊 Average CPU: " (count > 0 ? totalcpu/count : 0) "%"
        print "📊 Average Memory: " (count > 0 ? totalmem/count : 0) "%"
    }' "$LOG_FILE"
}

case "$1" in
    start)
        start_monitoring
        ;;
    stop)
        stop_monitoring
        ;;
    status)
        show_status
        ;;
    view)
        view_results
        ;;
    help|--help|-h)
        show_help
        ;;
    *)
        echo "❌ Unknown command: $1"
        echo ""
        show_help
        exit 1
        ;;
esac