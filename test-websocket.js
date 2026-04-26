#!/usr/bin/env node

const WebSocket = require('ws');

console.log('Testing WebSocket connection to statistics service...');

const ws = new WebSocket('ws://localhost:8006/api/v1/statistics/ws');

ws.on('open', function open() {
  console.log('✅ WebSocket connected successfully!');
  console.log('Waiting for statistics updates...');
});

ws.on('message', function message(data) {
  try {
    const parsed = JSON.parse(data);
    console.log('📊 Received statistics update:', parsed.type);
    if (parsed.data && parsed.data.type) {
      console.log('   Update type:', parsed.data.type);
    }
  } catch (error) {
    console.log('📨 Received message:', data.toString());
  }
});

ws.on('error', function error(err) {
  console.log('❌ WebSocket error:', err.message);
});

ws.on('close', function close() {
  console.log('🔌 WebSocket connection closed');
});

// Keep the connection alive for 10 seconds
setTimeout(() => {
  console.log('Closing connection...');
  ws.close();
}, 10000);