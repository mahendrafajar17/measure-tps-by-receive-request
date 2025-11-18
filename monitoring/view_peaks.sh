#!/bin/bash

LOG_FILE="reqforwarder_peaks.log"

if [ ! -f "$LOG_FILE" ]; then
    echo "Log file not found: $LOG_FILE"
    echo "Run ./monitor.sh first to start monitoring"
    exit 1
fi

echo "=== REQFORWARDER PEAK MONITORING RESULTS ==="
echo ""

# Total records
TOTAL_RECORDS=$(tail -n +2 "$LOG_FILE" | wc -l)
echo "Total monitoring records: $TOTAL_RECORDS"

if [ $TOTAL_RECORDS -eq 0 ]; then
    echo "No monitoring data available yet."
    exit 0
fi

echo ""

# Peak CPU
echo "=== TOP 5 PEAK CPU USAGE ==="
echo "Timestamp,CPU%,Memory%,MemoryMB"
sort -t, -k2 -nr "$LOG_FILE" | tail -n +2 | head -5

echo ""

# Peak Memory
echo "=== TOP 5 PEAK MEMORY USAGE ==="
echo "Timestamp,CPU%,Memory%,MemoryMB"
sort -t, -k3 -nr "$LOG_FILE" | tail -n +2 | head -5

echo ""

# Summary
echo "=== SUMMARY ==="
awk -F, 'NR>1 {
    if($2>maxcpu) {maxcpu=$2; maxcpu_time=$1}
    if($3>maxmem) {maxmem=$3; maxmem_time=$1}
    if($4>maxmem_mb) {maxmem_mb=$4; maxmem_mb_time=$1}
    totalcpu+=$2; totalmem+=$3; count++
} END {
    print "Peak CPU: " maxcpu "% at " maxcpu_time
    print "Peak Memory: " maxmem "% at " maxmem_time  
    print "Peak Memory MB: " maxmem_mb " MB at " maxmem_mb_time
    print "Average CPU: " (count > 0 ? totalcpu/count : 0) "%"
    print "Average Memory: " (count > 0 ? totalmem/count : 0) "%"
}' "$LOG_FILE"

echo ""
echo "=== CURRENT STATUS ==="
if pgrep -f "main.go|reqforwarder" > /dev/null; then
    echo "✅ Reqforwarder is running"
    ps aux | grep -E "main.go|reqforwarder" | grep -v grep | head -1 | awk '{print "Current CPU: " $3 "%, Memory: " $4 "%, Memory MB: " $6/1024}'
else
    echo "❌ Reqforwarder is not running"
fi

if [ -f "monitor.pid" ]; then
    if kill -0 $(cat monitor.pid 2>/dev/null) 2>/dev/null; then
        echo "✅ Monitor is running (PID: $(cat monitor.pid))"
    else
        echo "❌ Monitor is not running"
    fi
else
    echo "❌ Monitor PID file not found"
fi