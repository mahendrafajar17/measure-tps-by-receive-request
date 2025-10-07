#!/bin/bash

BASE_URL="http://localhost:8080"
REFRESH_INTERVAL=5

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
WHITE='\033[1;37m'
NC='\033[0m' # No Color

# Function to clear screen
clear_screen() {
    clear
}

# Function to print header
print_header() {
    echo -e "${WHITE}╔══════════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${WHITE}║                    ${CYAN}🎯 Webhook TPS Monitor${WHITE}                        ║${NC}"
    echo -e "${WHITE}║                     ${YELLOW}$(date '+%Y-%m-%d %H:%M:%S')${WHITE}                      ║${NC}"
    echo -e "${WHITE}╚══════════════════════════════════════════════════════════════════════╝${NC}"
    echo ""
}

# Function to check server status
check_server() {
    if curl -s "$BASE_URL/api/webhooks" > /dev/null 2>&1; then
        echo -e "${GREEN}✅ Server Status: ONLINE${NC}"
        return 0
    else
        echo -e "${RED}❌ Server Status: OFFLINE${NC}"
        echo -e "${YELLOW}Please start the server with: ./launch.sh start${NC}"
        return 1
    fi
}

# Function to format TPS with colors
format_tps() {
    local tps=$1
    local tps_int=$(echo "$tps" | cut -d'.' -f1)
    
    if (( $(echo "$tps_int >= 100" | bc -l) )); then
        echo -e "${GREEN}${tps}${NC}"
    elif (( $(echo "$tps_int >= 10" | bc -l) )); then
        echo -e "${YELLOW}${tps}${NC}"
    elif (( $(echo "$tps_int >= 1" | bc -l) )); then
        echo -e "${BLUE}${tps}${NC}"
    else
        echo -e "${RED}${tps}${NC}"
    fi
}

# Function to format delay with colors
format_delay() {
    local delay=$1
    
    if [ "$delay" -eq 0 ]; then
        echo -e "${GREEN}${delay}ms${NC}"
    elif [ "$delay" -le 500 ]; then
        echo -e "${YELLOW}${delay}ms${NC}"
    else
        echo -e "${RED}${delay}ms${NC}"
    fi
}

# Function to display webhook metrics
display_metrics() {
    echo -e "${WHITE}┌─────────────────────────────────────────────────────────────────────┐${NC}"
    echo -e "${WHITE}│ Webhook Name          │ Requests │    TPS    │  Delay  │ Duration   │${NC}"
    echo -e "${WHITE}├─────────────────────────────────────────────────────────────────────┤${NC}"
    
    # Get summary data
    local summary=$(curl -s "$BASE_URL/api/summary" 2>/dev/null)
    
    if [ $? -ne 0 ] || [ -z "$summary" ]; then
        echo -e "${WHITE}│ ${RED}Error fetching metrics data${NC}                                      │"
        echo -e "${WHITE}└─────────────────────────────────────────────────────────────────────┘${NC}"
        return
    fi
    
    # Parse and display each webhook
    echo "$summary" | jq -r '.summary | to_entries[] | "\(.key)|\(.value.name)|\(.value.total_requests)|\(.value.tps)|\(.value.delay_ms)|\(.value.duration_seconds)"' | while IFS='|' read -r id name requests tps delay duration; do
        
        # Format values
        local formatted_tps=$(format_tps "$tps")
        local formatted_delay=$(format_delay "$delay")
        local formatted_duration=$(printf "%.2fs" "$duration")
        
        # Truncate name if too long
        local display_name=$(echo "$name" | cut -c1-20)
        
        # Print row
        printf "${WHITE}│${NC} %-20s ${WHITE}│${NC} %8s ${WHITE}│${NC} %9s ${WHITE}│${NC} %7s ${WHITE}│${NC} %10s ${WHITE}│${NC}\n" \
               "$display_name" "$requests" "$formatted_tps" "$formatted_delay" "$formatted_duration"
    done
    
    echo -e "${WHITE}└─────────────────────────────────────────────────────────────────────┘${NC}"
}

# Function to display quick stats
display_quick_stats() {
    echo ""
    echo -e "${WHITE}📊 Quick Stats:${NC}"
    
    local summary=$(curl -s "$BASE_URL/api/summary" 2>/dev/null)
    local total_requests=0
    local active_webhooks=0
    local fastest_tps=0
    local fastest_name=""
    
    if [ $? -eq 0 ] && [ -n "$summary" ]; then
        while IFS='|' read -r name requests tps; do
            total_requests=$((total_requests + requests))
            if [ "$requests" -gt 0 ]; then
                active_webhooks=$((active_webhooks + 1))
            fi
            if (( $(echo "$tps > $fastest_tps" | bc -l) )); then
                fastest_tps="$tps"
                fastest_name="$name"
            fi
        done < <(echo "$summary" | jq -r '.summary | to_entries[] | "\(.value.name)|\(.value.total_requests)|\(.value.tps)"')
        
        echo -e "   ${CYAN}Total Requests:${NC} $total_requests"
        echo -e "   ${CYAN}Active Webhooks:${NC} $active_webhooks"
        if [ -n "$fastest_name" ] && (( $(echo "$fastest_tps > 0" | bc -l) )); then
            echo -e "   ${CYAN}Fastest:${NC} $fastest_name ($(format_tps "$fastest_tps") TPS)"
        fi
    fi
}

# Function to show available commands
show_commands() {
    echo ""
    echo -e "${WHITE}💡 Available Commands:${NC}"
    echo -e "   ${CYAN}r${NC} - Reset all metrics"
    echo -e "   ${CYAN}q${NC} - Quit monitor"
    echo -e "   ${CYAN}Enter${NC} - Refresh now"
}

# Function to reset all metrics
reset_metrics() {
    echo -e "\n${YELLOW}Resetting all webhook metrics...${NC}"
    curl -s -X POST "$BASE_URL/api/webhooks/default/reset" > /dev/null
    curl -s -X POST "$BASE_URL/api/webhooks/fast/reset" > /dev/null
    curl -s -X POST "$BASE_URL/api/webhooks/slow/reset" > /dev/null
    curl -s -X POST "$BASE_URL/api/webhooks/medium/reset" > /dev/null
    curl -s -X POST "$BASE_URL/api/webhooks/custom/reset" > /dev/null
    echo -e "${GREEN}✅ All metrics reset${NC}"
    sleep 1
}

# Function to handle user input
handle_input() {
    read -t 0.1 -n 1 key 2>/dev/null
    case "$key" in
        'r'|'R')
            reset_metrics
            ;;
        'q'|'Q')
            clear_screen
            echo -e "${GREEN}👋 Monitor stopped${NC}"
            exit 0
            ;;
        '')
            # Enter key pressed or timeout - refresh display
            ;;
    esac
}

# Main monitoring loop
main() {
    echo -e "${WHITE}Starting Webhook TPS Monitor...${NC}"
    echo -e "${YELLOW}Press 'r' to reset metrics, 'q' to quit, Enter to refresh${NC}"
    sleep 2
    
    while true; do
        clear_screen
        print_header
        
        if check_server; then
            echo ""
            display_metrics
            display_quick_stats
            show_commands
            
            echo ""
            echo -e "${PURPLE}Auto-refresh in $REFRESH_INTERVAL seconds... (Press key for commands)${NC}"
        else
            echo -e "\n${RED}Cannot connect to webhook server${NC}"
            echo -e "${YELLOW}Make sure the server is running on $BASE_URL${NC}"
        fi
        
        # Wait for input or timeout
        for i in $(seq 1 $REFRESH_INTERVAL); do
            handle_input
            sleep 1
        done
    done
}

# Check if jq is available
if ! command -v jq &> /dev/null; then
    echo -e "${RED}Error: jq is required but not installed${NC}"
    echo -e "${YELLOW}Please install jq: brew install jq (macOS) or apt-get install jq (Ubuntu)${NC}"
    exit 1
fi

# Check if bc is available
if ! command -v bc &> /dev/null; then
    echo -e "${RED}Error: bc is required but not installed${NC}"
    echo -e "${YELLOW}Please install bc: brew install bc (macOS) or apt-get install bc (Ubuntu)${NC}"
    exit 1
fi

# Trap Ctrl+C
trap 'clear_screen; echo -e "\n${GREEN}👋 Monitor stopped${NC}"; exit 0' INT

# Run main function
main