/**
 * OOBE Setup Route
 * Handles first-time server initialization.
 * Saves config to server.config.json and getui.config.json,
 * then marks the server as initialized.
 */

const express = require('express');
const fs = require('fs');
const path = require('path');
const crypto = require('crypto');

const router = express.Router();
const CONFIG_PATH = path.join(__dirname, '..', 'server.config.json');

// Check if server has been initialized
function isInitialized() {
  try {
    if (fs.existsSync(CONFIG_PATH)) {
      const config = JSON.parse(fs.readFileSync(CONFIG_PATH, 'utf-8'));
      return config.initialized === true;
    }
  } catch {}
  return false;
}

// Load server config
function loadConfig() {
  try {
    if (fs.existsSync(CONFIG_PATH)) {
      return JSON.parse(fs.readFileSync(CONFIG_PATH, 'utf-8'));
    }
  } catch {}
  return null;
}

// Save server config
function saveConfig(config) {
  try {
    const existingConfig = loadConfig() || {};
    const updatedConfig = { ...existingConfig, ...config };
    fs.writeFileSync(CONFIG_PATH, JSON.stringify(updatedConfig, null, 2));
    return true;
  } catch (err) {
    console.error('[Config] Failed to save config:', err.message);
    return false;
  }
}

// Check initialization status
router.get('/status', (req, res) => {
  res.json({ initialized: isInitialized() });
});

// Save setup configuration
router.post('/', (req, res) => {
  // Only allow setup if not already initialized
  if (isInitialized()) {
    return res.status(403).json({ error: '服务器已初始化，如需修改请在管理后台操作' });
  }

  const {
    adminUser = 'admin',
    adminPass,
    serverName = 'HutaomuChat',
    serverPort = 3000,
    jwtSecret = '',
    getuiAppId = '',
    getuiAppKey = '',
    getuiMasterSecret = ''
  } = req.body;

  if (!adminPass || adminPass.length < 6) {
    return res.status(400).json({ error: '管理员密码至少 6 个字符' });
  }

  // Generate JWT secret if not provided
  const finalJwtSecret = jwtSecret || crypto.randomBytes(32).toString('hex');

  // Save server config
  const serverConfig = {
    initialized: true,
    adminUser,
    adminPass,
    serverName,
    serverPort,
    jwtSecret: finalJwtSecret,
    setupTime: new Date().toISOString()
  };

  try {
    fs.writeFileSync(CONFIG_PATH, JSON.stringify(serverConfig, null, 2));
    console.log('[OOBE] Server config saved');
  } catch (e) {
    return res.status(500).json({ error: '写入配置文件失败: ' + e.message });
  }

  // Save GeTui config if provided
  if (getuiAppId && getuiAppKey && getuiMasterSecret) {
    const getuiConfig = {
      appId: getuiAppId,
      appKey: getuiAppKey,
      masterSecret: getuiMasterSecret
    };
    try {
      const getuiPath = path.join(__dirname, '..', 'getui.config.json');
      fs.writeFileSync(getuiPath, JSON.stringify(getuiConfig, null, 2));
      console.log('[OOBE] GeTui config saved');
    } catch (e) {
      console.error('[OOBE] Failed to save GeTui config:', e.message);
    }
  }

  res.json({ success: true, message: '初始化完成，请重启服务端使配置生效' });
});

module.exports = { router, isInitialized, loadConfig, saveConfig };
