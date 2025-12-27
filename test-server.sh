#!/bin/bash
# Simple test script for the server

echo "=== Testing StackArr Server ==="
echo ""
echo "1. Starting server in background..."
cd /mnt/c/Dev/stackarr
go run cmd/stackarr/main.go &
SERVER_PID=$!
echo "Server PID: $SERVER_PID"

echo ""
echo "2. Waiting 3 seconds for server to start..."
sleep 3

echo ""
echo "3. Checking if server is running..."
if ps -p $SERVER_PID > /dev/null; then
    echo "✓ Server process is running"
else
    echo "✗ Server process died"
    exit 1
fi

echo ""
echo "4. Testing with curl..."
curl -v http://127.0.0.1:8080

echo ""
echo ""
echo "5. Stopping server..."
kill $SERVER_PID

echo "Done!"
