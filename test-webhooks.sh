#!/bin/bash

BASE_URL="http://localhost:8080"

echo "=== Testing Webhook Server TPS Metrics ==="
echo ""

# Check if server is running
echo "1. Checking if server is running..."
if ! curl -s "$BASE_URL/api/webhooks" > /dev/null; then
    echo "❌ Server is not running. Please start with: ./launch.sh start"
    exit 1
fi
echo "✅ Server is running"
echo ""

# Reset all metrics first
echo "2. Resetting all metrics..."
curl -s -X POST "$BASE_URL/api/webhooks/fast/reset" > /dev/null
curl -s -X POST "$BASE_URL/api/webhooks/slow/reset" > /dev/null
curl -s -X POST "$BASE_URL/api/webhooks/default/reset" > /dev/null
echo "✅ All metrics reset"
echo ""

# Test Fast Webhook (0ms delay)
echo "3. Testing FAST webhook (0ms delay) - 10 requests..."
start_time=$(date +%s%N)
for i in {1..10}; do
    curl -s "$BASE_URL/webhook/fast" > /dev/null
done
end_time=$(date +%s%N)
duration_ms=$(( (end_time - start_time) / 1000000 ))
echo "⚡ 10 fast requests completed in ${duration_ms}ms"
echo ""

# Get fast webhook metrics
echo "📊 Fast Webhook Metrics:"
curl -s "$BASE_URL/api/webhooks/fast/metrics" | jq '{
  total_requests: .total_requests,
  duration_seconds: .duration_seconds,
  tps: .tps,
  delay: "0ms"
}'
echo ""

# Test Slow Webhook (2000ms delay)
echo "4. Testing SLOW webhook (2000ms delay) - 3 requests..."
start_time=$(date +%s%N)
for i in {1..3}; do
    echo "   Request $i..."
    curl -s "$BASE_URL/webhook/slow" > /dev/null
done
end_time=$(date +%s%N)
duration_ms=$(( (end_time - start_time) / 1000000 ))
echo "🐌 3 slow requests completed in ${duration_ms}ms"
echo ""

# Get slow webhook metrics
echo "📊 Slow Webhook Metrics:"
curl -s "$BASE_URL/api/webhooks/slow/metrics" | jq '{
  total_requests: .total_requests,
  duration_seconds: .duration_seconds,
  tps: .tps,
  delay: "2000ms"
}'
echo ""

# Test Medium Webhook (500ms delay) 
echo "5. Testing MEDIUM webhook (500ms delay) - 5 requests..."
start_time=$(date +%s%N)
for i in {1..5}; do
    curl -s "$BASE_URL/webhook/medium" > /dev/null
done
end_time=$(date +%s%N)
duration_ms=$(( (end_time - start_time) / 1000000 ))
echo "⏱️  5 medium requests completed in ${duration_ms}ms"
echo ""

# Get medium webhook metrics
echo "📊 Medium Webhook Metrics:"
curl -s "$BASE_URL/api/webhooks/medium/metrics" | jq '{
  total_requests: .total_requests,
  duration_seconds: .duration_seconds,
  tps: .tps,
  delay: "500ms"
}'
echo ""

# Summary of all webhooks
echo "6. 📊 SUMMARY - All Webhook Metrics:"
echo ""
echo "Fast (0ms delay):"
curl -s "$BASE_URL/api/webhooks/fast/metrics" | jq -r '"  Requests: \(.total_requests), TPS: \(.tps | floor), Duration: \(.duration_seconds)s"'

echo "Medium (500ms delay):"
curl -s "$BASE_URL/api/webhooks/medium/metrics" | jq -r '"  Requests: \(.total_requests), TPS: \(.tps | floor), Duration: \(.duration_seconds)s"'

echo "Slow (2000ms delay):"
curl -s "$BASE_URL/api/webhooks/slow/metrics" | jq -r '"  Requests: \(.total_requests), TPS: \(.tps | floor), Duration: \(.duration_seconds)s"'

echo ""
echo "=== Test Complete ==="

# Bonus: Test concurrent requests
echo ""
echo "7. 🚀 BONUS: Testing concurrent requests to fast webhook..."
echo "Sending 20 concurrent requests..."

# Create background jobs
for i in {1..20}; do
    curl -s "$BASE_URL/webhook/fast" > /dev/null &
done

# Wait for all background jobs to complete
wait

echo "✅ All concurrent requests completed"
echo ""
echo "📊 Updated Fast Webhook Metrics (after concurrent test):"
curl -s "$BASE_URL/api/webhooks/fast/metrics" | jq '{
  total_requests: .total_requests,
  duration_seconds: .duration_seconds,
  tps: .tps
}'