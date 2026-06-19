const express = require('express');
const cors = require('cors');
const path = require('path');
const http = require('http');
const fs = require('fs');

const db = require('./db');
const logger = require('./logger');
const { router: setupRouter, isInitialized, loadConfig } = require('./routes/setup');

const app = express();

app.use(cors());
app.use(express.json());
app.use('/uploads', express.static(path.join(__dirname, 'uploads')));

// ===== OOBE Setup Check (must be first) =====
app.use('/api/setup', setupRouter);
app.use('/admin/setup', (req, res) => {
  res.sendFile(path.join(__dirname, 'admin', 'setup.html'));
});

// If not initialized, redirect everything to setup
app.use((req, res, next) => {
  if (!isInitialized()) {
    // Allow setup-related requests through
    if (req.path.startsWith('/api/setup') || req.path.startsWith('/admin/setup') || req.path === '/api/health') {
      return next();
    }
    // Redirect to setup page
    return res.redirect('/admin/setup');
  }
  next();
});

// ===== Load server config (after initialization) =====
const serverConfig = loadConfig();
if (serverConfig) {
  // Apply admin credentials
  process.env.ADMIN_USER = serverConfig.adminUser || 'admin';
  process.env.ADMIN_PASS = serverConfig.adminPass || 'hutaomu2024';
  // Apply JWT secret
  if (serverConfig.jwtSecret) {
    process.env.JWT_SECRET = serverConfig.jwtSecret;
  }
  console.log(`[Config] Server: ${serverConfig.serverName || 'HutaomuChat'}, Admin: ${serverConfig.adminUser}`);
}

const WebSocketManager = require('./ws');

const authRoutes = require('./routes/auth');
const userRoutes = require('./routes/users');
const friendRoutes = require('./routes/friends');
const chatRoutes = require('./routes/chats');
const messageRoutes = require('./routes/messages');
const momentRoutes = require('./routes/moments');
const uploadRoutes = require('./routes/upload');
const pushRoutes = require('./routes/push');
const adminRoutes = require('./routes/admin');

const server = http.createServer(app);
const PORT = process.env.PORT || (serverConfig && serverConfig.serverPort) || 3000;

// REST API Routes
app.use('/api/auth', authRoutes);
app.use('/api/users', userRoutes);
app.use('/api/friends', friendRoutes);
app.use('/api/chats', chatRoutes);
app.use('/api/messages', messageRoutes);
app.use('/api/moments', momentRoutes);
app.use('/api/upload', uploadRoutes);
app.use('/api/push', pushRoutes);
app.use('/api/stickers', require('./routes/stickers'));
app.use('/api/admin', adminRoutes);
app.use('/admin', express.static(path.join(__dirname, 'admin')));

// Health check
app.get('/api/health', (req, res) => {
  res.json({ status: 'ok', timestamp: new Date().toISOString(), initialized: isInitialized() });
});

// WebSocket
const wsManager = new WebSocketManager(server);
db.wsManager = wsManager;

// Auto-cleanup: delete uploaded files older than 7 days
const UPLOADS_DIR = path.join(__dirname, 'uploads');
const SEVEN_DAYS_MS = 7 * 24 * 60 * 60 * 1000;

function cleanupOldUploads() {
  try {
    if (!fs.existsSync(UPLOADS_DIR)) return;
    const now = Date.now();
    const files = fs.readdirSync(UPLOADS_DIR);
    let deleted = 0;
    files.forEach(file => {
      const filePath = path.join(UPLOADS_DIR, file);
      const stat = fs.statSync(filePath);
      if (now - stat.mtimeMs > SEVEN_DAYS_MS) {
        fs.unlinkSync(filePath);
        deleted++;
      }
    });
    if (deleted > 0) console.log(`Cleanup: deleted ${deleted} old upload(s)`);
  } catch (err) {
    console.error('Cleanup error:', err.message);
  }
}

cleanupOldUploads();
setInterval(cleanupOldUploads, 24 * 60 * 60 * 1000);

// Download endpoint
app.get('/api/download/:filename', (req, res) => {
  const filename = req.params.filename;
  const originalName = req.query.name || filename;
  const filePath = path.join(__dirname, 'uploads', filename);
  if (!fs.existsSync(filePath)) {
    return res.status(404).json({ error: 'File not found' });
  }
  const encodedName = encodeURIComponent(originalName);
  res.setHeader('Content-Disposition', `attachment; filename*=UTF-8''${encodedName}`);
  res.sendFile(filePath);
});

server.listen(PORT, () => {
  const name = (serverConfig && serverConfig.serverName) || 'HutaomuChat';
  console.log('');
  console.log(`${name} Server running on:`);
  console.log(`  HTTP:  http://0.0.0.0:${PORT}`);
  console.log(`  WS:    ws://0.0.0.0:${PORT}`);
  if (!isInitialized()) {
    console.log(`  SETUP: http://0.0.0.0:${PORT}/admin/setup (first-time setup)`);
  }
  console.log('');
  console.log('  HutaomuChat Server - AGPL-3.0 License');
  console.log('  Copyright (C) 2026 qiuxi903');
  console.log('  Source: https://github.com/qiuxi903/hutaomuchat-server');
  console.log('');
  console.log('  This program is free software: you can redistribute it and/or modify');
  console.log('  it under the terms of the GNU Affero General Public License v3.0.');
  console.log('');

  // Log server start
  logger.system.info(`${name} Server started`, { port: PORT, initialized: isInitialized() });
});
