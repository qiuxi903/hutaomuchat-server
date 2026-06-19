/**
 * GeTui UniPush 2.0 REST API Module
 * https://docs.getui.com/getui/server/rest_v2/push/
 *
 * When a user is offline (WebSocket disconnected), the server sends a push
 * notification through GeTui's system-level push channel. This works even
 * when the app process is completely killed, because the push is delivered
 * by the device manufacturer's system service (OPPO Push, vivo Push, etc.).
 *
 * Config: create getui.config.json in the same directory with:
 * {
 *   "appId": "your-app-id",
 *   "appKey": "your-app-key",
 *   "masterSecret": "your-master-secret"
 * }
 *
 * Get these from https://dev.getui.com after registering your app.
 */

const https = require('https');
const crypto = require('crypto');
const fs = require('fs');
const path = require('path');

const BASE_URL = 'https://restapi.getui.com/v2';

// Load config
let config = null;
try {
  const configPath = path.join(__dirname, 'getui.config.json');
  config = JSON.parse(fs.readFileSync(configPath, 'utf-8'));
  console.log('[GeTui] Config loaded, appId:', config.appId);
} catch (e) {
  console.log('[GeTui] No config file found. Push notifications disabled.');
  console.log('[GeTui] Create getui.config.json with { appId, appKey, masterSecret }');
}

let authToken = null;
let tokenExpiry = 0;

/**
 * Make an HTTPS request to GeTui REST API
 */
function request(method, endpoint, body = null) {
  return new Promise((resolve, reject) => {
    const url = new URL(BASE_URL + endpoint);
    const headers = {
      'Content-Type': 'application/json',
    };
    if (authToken) {
      headers['token'] = authToken;
    }

    const bodyStr = body ? JSON.stringify(body) : null;
    if (bodyStr) {
      headers['Content-Length'] = Buffer.byteLength(bodyStr);
    }

    const options = {
      hostname: url.hostname,
      port: 443,
      path: url.pathname + url.search,
      method,
      headers,
    };

    const req = https.request(options, (res) => {
      let data = '';
      res.on('data', (chunk) => { data += chunk; });
      res.on('end', () => {
        try {
          resolve(JSON.parse(data));
        } catch (e) {
          reject(new Error(`Invalid JSON response: ${data}`));
        }
      });
    });

    req.on('error', reject);
    if (bodyStr) req.write(bodyStr);
    req.end();
  });
}

/**
 * Authenticate with GeTui and get an auth token (valid for 24 hours)
 */
async function authenticate() {
  if (!config) return false;
  if (authToken && Date.now() < tokenExpiry) return true;

  const timestamp = Date.now().toString();
  const sign = crypto
    .createHash('sha256')
    .update(config.appKey + timestamp + config.masterSecret)
    .digest('hex');

  try {
    const result = await request('POST', `/${config.appId}/auth`, {
      sign,
      timestamp,
      appkey: config.appKey,
    });

    if (result.code === 0 && result.data) {
      authToken = result.data.token;
      // Token expires in 24 hours, refresh 1 hour early
      tokenExpiry = Date.now() + (result.data.expire_time || 86400000) - 3600000;
      console.log('[GeTui] Authenticated successfully');
      return true;
    } else {
      console.error('[GeTui] Auth failed:', result.msg);
      return false;
    }
  } catch (e) {
    console.error('[GeTui] Auth error:', e.message);
    return false;
  }
}

/**
 * Send a push notification to a single user via their CID (Client ID)
 * @param {string} cid - GeTui client ID (registered by the Android client)
 * @param {string} title - Notification title
 * @param {string} body - Notification body text
 * @param {object} payload - Extra data (e.g., { chatId: "xxx" })
 */
async function sendPushToSingle(cid, title, body, payload = {}) {
  if (!config) return false;

  const authenticated = await authenticate();
  if (!authenticated) return false;

  try {
    // Step 1: Create a push message (get task_id)
    const createResult = await request('POST', `/${config.appId}/push/list/message`, {
      request_id: crypto.randomUUID().replace(/-/g, ''),
      group_name: 'hutaomuchat_msg',
      push_message: {
        notification: {
          title,
          body,
          click_type: 'startapp', // Open app on click
          payload: JSON.stringify(payload),
        },
      },
    });

    if (createResult.code !== 0 || !createResult.data) {
      console.error('[GeTui] Create push failed:', createResult.msg);
      return false;
    }

    const taskId = createResult.data.task_id;

    // Step 2: Send to the specific CID
    const sendResult = await request('POST', `/${config.appId}/push/list/cid`, {
      audience: {
        cid: [cid],
      },
      taskid: taskId,
      is_async: true,
    });

    if (sendResult.code === 0) {
      console.log(`[GeTui] Push sent to ${cid.substring(0, 8)}... : "${title}"`);
      return true;
    } else {
      console.error('[GeTui] Send push failed:', sendResult.msg);
      return false;
    }
  } catch (e) {
    console.error('[GeTui] Push error:', e.message);
    return false;
  }
}

/**
 * Check if push is configured and available
 */
function isAvailable() {
  return config !== null && config.appId && config.appKey && config.masterSecret;
}

module.exports = {
  sendPushToSingle,
  isAvailable,
  authenticate,
};
