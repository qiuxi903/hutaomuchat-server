const express = require('express');
const jwt = require('jsonwebtoken');
const db = require('../db');
const { JWT_SECRET } = require('../middleware/auth');
const path = require('path');
const fs = require('fs');

const router = express.Router();

// Admin credentials (from env or defaults)
const ADMIN_USER = process.env.ADMIN_USER || 'admin';
const ADMIN_PASS = process.env.ADMIN_PASS || 'hutaomu2024';

// Admin auth middleware
function adminAuth(req, res, next) {
  const authHeader = req.headers['authorization'];
  if (!authHeader) return res.status(401).json({ error: '需要管理员认证' });
  const token = authHeader.startsWith('Bearer ') ? authHeader.slice(7) : authHeader;
  try {
    const decoded = jwt.verify(token, JWT_SECRET);
    if (decoded.role !== 'admin') return res.status(403).json({ error: '权限不足' });
    req.admin = decoded;
    next();
  } catch {
    return res.status(401).json({ error: '认证已过期' });
  }
}

// Admin login
router.post('/login', (req, res) => {
  const { username, password } = req.body;
  if (username === ADMIN_USER && password === ADMIN_PASS) {
    const token = jwt.sign({ role: 'admin', username }, JWT_SECRET, { expiresIn: '24h' });
    return res.json({ success: true, token });
  }
  res.status(401).json({ error: '用户名或密码错误' });
});

// ========== Dashboard Stats ==========
router.get('/stats', adminAuth, (req, res) => {
  const now = Date.now();
  const todayStart = new Date();
  todayStart.setHours(0, 0, 0, 0);
  const todayMs = todayStart.getTime();

  const onlineUsers = db.wsManager ? db.wsManager.clients.size : 0;
  const totalUsers = db.users.length;
  const totalChats = db.chats.length;
  const totalGroups = db.chats.filter(c => c.type === 'GROUP').length;
  const totalMessages = db.messages.length;
  const todayMessages = db.messages.filter(m => m.timestamp >= todayMs).length;
  const totalMoments = db.moments.length;
  const todayMoments = db.moments.filter(m => m.created_at >= todayMs).length;
  const totalFriendships = db.friendships.length;
  const pendingJoinRequests = db.groupJoinRequests.filter(r => r.status === 'PENDING').length;
  const offlineQueueSize = db.offlineQueue.length;
  const totalStickers = db.stickers.length;

  // Messages per day (last 7 days)
  const dailyMessages = [];
  for (let i = 6; i >= 0; i--) {
    const day = new Date();
    day.setHours(0, 0, 0, 0);
    day.setDate(day.getDate() - i);
    const dayStart = day.getTime();
    const dayEnd = dayStart + 86400000;
    const count = db.messages.filter(m => m.timestamp >= dayStart && m.timestamp < dayEnd).length;
    dailyMessages.push({ date: day.toISOString().split('T')[0], count });
  }

  // User registrations per day (last 7 days)
  const dailyRegistrations = [];
  for (let i = 6; i >= 0; i--) {
    const day = new Date();
    day.setHours(0, 0, 0, 0);
    day.setDate(day.getDate() - i);
    const dayStart = day.getTime();
    const dayEnd = dayStart + 86400000;
    const count = db.users.filter(u => u.created_at >= dayStart && u.created_at < dayEnd).length;
    dailyRegistrations.push({ date: day.toISOString().split('T')[0], count });
  }

  // Message type distribution
  const msgTypes = {};
  db.messages.forEach(m => {
    const t = m.type || 'TEXT';
    msgTypes[t] = (msgTypes[t] || 0) + 1;
  });

  res.json({
    onlineUsers,
    totalUsers,
    totalChats,
    totalGroups,
    totalMessages,
    todayMessages,
    totalMoments,
    todayMoments,
    totalFriendships,
    pendingJoinRequests,
    offlineQueueSize,
    totalStickers,
    dailyMessages,
    dailyRegistrations,
    msgTypes,
  });
});

// ========== User Management ==========
router.get('/users', adminAuth, (req, res) => {
  const { search, page = 1, pageSize = 20 } = req.query;
  let users = [...db.users];
  if (search) {
    const q = search.toLowerCase();
    users = users.filter(u =>
      (u.nickname || '').toLowerCase().includes(q) ||
      (u.phone || '').includes(q) ||
      (u.uid || '').includes(q) ||
      (u.id || '').includes(q)
    );
  }
  const total = users.length;
  const offset = (parseInt(page) - 1) * parseInt(pageSize);
  const paged = users.slice(offset, offset + parseInt(pageSize));

  // Enrich with online status and push token
  const enriched = paged.map(u => ({
    ...u,
    password: undefined, // strip password
    isOnline: db.wsManager ? db.wsManager.clients.has(u.id) : false,
    hasPushToken: !!db.getPushToken(u.id),
    friendCount: db.friendships.filter(f => f.user_id === u.id || f.friend_id === u.id).length,
    chatCount: db.chatMembers.filter(cm => cm.user_id === u.id).length,
  }));

  res.json({ users: enriched, total, page: parseInt(page), pageSize: parseInt(pageSize) });
});

router.get('/users/:id', adminAuth, (req, res) => {
  const user = db.findUserById(req.params.id);
  if (!user) return res.status(404).json({ error: '用户不存在' });
  const { password, ...safeUser } = user;
  const friends = db.getFriends(user.id).map(fid => {
    const f = db.findUserById(fid);
    return f ? { id: f.id, uid: f.uid, nickname: f.nickname, avatar: f.avatar_url } : null;
  }).filter(Boolean);
  const chats = db.chatMembers.filter(cm => cm.user_id === user.id).map(cm => {
    const chat = db.getChat(cm.chat_id);
    return chat ? { id: chat.id, name: chat.name, type: chat.type, role: cm.role } : null;
  }).filter(Boolean);

  res.json({ user: safeUser, friends, chats });
});

router.put('/users/:id', adminAuth, (req, res) => {
  const { nickname, signature, avatar_url, background_url, background_original_url, phone, banned } = req.body;
  const updates = {};
  if (nickname !== undefined) updates.nickname = nickname;
  if (signature !== undefined) updates.signature = signature;
  if (avatar_url !== undefined) updates.avatar_url = avatar_url;
  if (background_url !== undefined) updates.background_url = background_url;
  if (background_original_url !== undefined) updates.background_original_url = background_original_url;
  if (phone !== undefined) updates.phone = phone;
  if (banned !== undefined) updates.banned = banned;
  const updated = db.updateUser(req.params.id, updates);
  if (!updated) return res.status(404).json({ error: '用户不存在' });

  // If user just got banned, force disconnect from WebSocket
  if (banned === true && db.wsManager) {
    db.wsManager.disconnectUser(req.params.id);
  }

  res.json({ success: true, user: { ...updated, password: undefined } });
});

router.delete('/users/:id', adminAuth, (req, res) => {
  const idx = db.users.findIndex(u => u.id === req.params.id);
  if (idx < 0) return res.status(404).json({ error: '用户不存在' });
  const userId = req.params.id;
  db.users.splice(idx, 1);
  // Clean up related data
  db.friendships = db.friendships.filter(f => f.user_id !== userId && f.friend_id !== userId);
  db.friendRequests = db.friendRequests.filter(f => f.from_user_id !== userId && f.to_user_id !== userId);
  db.chatMembers = db.chatMembers.filter(cm => cm.user_id !== userId);
  db.messages = db.messages.filter(m => m.sender_id !== userId);
  db.moments = db.moments.filter(m => m.user_id !== userId);
  db.momentLikes = db.momentLikes.filter(l => l.user_id !== userId);
  db.momentComments = db.momentComments.filter(c => c.user_id !== userId);
  db.stickers = db.stickers.filter(s => s.user_id !== userId);
  db.offlineQueue = db.offlineQueue.filter(q => q.recipient_id !== userId);
  db.pushTokens = db.pushTokens.filter(t => t.user_id !== userId);
  db.groupJoinRequests = db.groupJoinRequests.filter(r => r.from_user_id !== userId);
  res.json({ success: true });
});

// ========== Chat/Group Management ==========
router.get('/chats', adminAuth, (req, res) => {
  const { search, type, page = 1, pageSize = 20 } = req.query;
  let chats = [...db.chats];
  if (type) chats = chats.filter(c => c.type === type);
  if (search) {
    const q = search.toLowerCase();
    chats = chats.filter(c =>
      (c.name || '').toLowerCase().includes(q) ||
      (c.id || '').includes(q)
    );
  }
  const total = chats.length;
  const offset = (parseInt(page) - 1) * parseInt(pageSize);
  const paged = chats.slice(offset, offset + parseInt(pageSize));

  const enriched = paged.map(c => {
    const memberIds = db.getChatMembers(c.id);
    const msgCount = db.messages.filter(m => m.chat_id === c.id).length;
    const owner = c.owner_id ? db.findUserById(c.owner_id) : null;
    return {
      ...c,
      memberCount: memberIds.length,
      messageCount: msgCount,
      ownerName: owner ? owner.nickname : '系统',
      members: memberIds.map(uid => {
        const u = db.findUserById(uid);
        return u ? { id: u.id, uid: u.uid, nickname: u.nickname, avatar: u.avatar_url } : null;
      }).filter(Boolean),
    };
  });

  res.json({ chats: enriched, total, page: parseInt(page), pageSize: parseInt(pageSize) });
});

router.get('/chats/:id/messages', adminAuth, (req, res) => {
  const { page = 1, pageSize = 50 } = req.query;
  const messages = db.getMessages(req.params.id);
  const total = messages.length;
  const offset = (parseInt(page) - 1) * parseInt(pageSize);
  const paged = messages.slice(offset, offset + parseInt(pageSize));
  const enriched = paged.map(m => {
    const sender = db.findUserById(m.sender_id);
    return { ...m, senderName: sender ? sender.nickname : '未知' };
  });
  res.json({ messages: enriched, total, page: parseInt(page), pageSize: parseInt(pageSize) });
});

router.delete('/chats/:id', adminAuth, (req, res) => {
  db.removeChatById(req.params.id);
  res.json({ success: true });
});

// ========== Moments Management ==========
router.get('/moments', adminAuth, (req, res) => {
  const { page = 1, pageSize = 20 } = req.query;
  const moments = [...db.moments].sort((a, b) => b.created_at - a.created_at);
  const total = moments.length;
  const offset = (parseInt(page) - 1) * parseInt(pageSize);
  const paged = moments.slice(offset, offset + parseInt(pageSize));
  const enriched = paged.map(m => {
    const author = db.findUserById(m.user_id);
    const likes = db.getMomentLikes(m.id).map(l => {
      const liker = db.findUserById(l.user_id);
      return liker ? { id: liker.id, nickname: liker.nickname, avatar: liker.avatar_url } : null;
    }).filter(Boolean);
    const comments = db.getMomentComments(m.id).map(c => {
      const commenter = db.findUserById(c.user_id);
      return {
        ...c,
        authorName: commenter ? commenter.nickname : '未知',
        authorAvatar: commenter ? commenter.avatar_url : '',
      };
    });
    // Parse images JSON string if needed
    let images = m.images;
    if (typeof images === 'string') {
      try { images = JSON.parse(images); } catch { images = []; }
    }
    return {
      ...m,
      images: images || [],
      authorName: author ? author.nickname : '未知',
      authorAvatar: author ? author.avatar_url : '',
      likeCount: likes.length,
      commentCount: comments.length,
      likes,
      comments,
    };
  });
  res.json({ moments: enriched, total, page: parseInt(page), pageSize: parseInt(pageSize) });
});

router.delete('/moments/:id', adminAuth, (req, res) => {
  const idx = db.moments.findIndex(m => m.id === req.params.id);
  if (idx < 0) return res.status(404).json({ error: '动态不存在' });
  const momentId = req.params.id;
  db.moments.splice(idx, 1);
  db.momentLikes = db.momentLikes.filter(l => l.moment_id !== momentId);
  db.momentComments = db.momentComments.filter(c => c.moment_id !== momentId);
  res.json({ success: true });
});

// ========== Announcements Management ==========
router.get('/announcements', adminAuth, (req, res) => {
  const all = [...db.announcements].sort((a, b) => b.created_at - a.created_at);
  const enriched = all.map(a => {
    const creator = db.findUserById(a.created_by);
    const chat = db.getChat(a.chat_id);
    return {
      ...a,
      creatorName: creator ? creator.nickname : '系统',
      chatName: chat ? chat.name : '未知群组',
    };
  });
  res.json({ announcements: enriched });
});

router.delete('/announcements/:id', adminAuth, (req, res) => {
  const idx = db.announcements.findIndex(a => a.id === req.params.id);
  if (idx < 0) return res.status(404).json({ error: '公告不存在' });
  db.announcements.splice(idx, 1);
  res.json({ success: true });
});

// ========== System Info ==========
router.get('/system', adminAuth, (req, res) => {
  const uploadsDir = path.join(__dirname, '..', 'uploads');
  let uploadFiles = [];
  let uploadSizeTotal = 0;
  try {
    if (fs.existsSync(uploadsDir)) {
      const files = fs.readdirSync(uploadsDir);
      uploadFiles = files.map(f => {
        const stat = fs.statSync(path.join(uploadsDir, f));
        uploadSizeTotal += stat.size;
        return { name: f, size: stat.size, created: stat.mtimeMs };
      }).sort((a, b) => b.created - a.created);
    }
  } catch {}

  const uptime = process.uptime();
  const memUsage = process.memoryUsage();

  // GeTui push status
  let pushStatus = 'unconfigured';
  try {
    const getui = require('../getui');
    pushStatus = getui.isAvailable() ? 'active' : 'unconfigured';
  } catch { pushStatus = 'error'; }

  res.json({
    uptime: Math.floor(uptime),
    memory: {
      rss: memUsage.rss,
      heapUsed: memUsage.heapUsed,
      heapTotal: memUsage.heapTotal,
    },
    wsConnections: db.wsManager ? db.wsManager.clients.size : 0,
    uploadFiles: uploadFiles.slice(0, 100),
    uploadFileCount: uploadFiles.length,
    uploadSizeTotal,
    pushStatus,
    nodeVersion: process.version,
    platform: process.platform,
  });
});

// Delete an uploaded file
router.delete('/system/files/:filename', adminAuth, (req, res) => {
  const filePath = path.join(__dirname, '..', 'uploads', req.params.filename);
  if (fs.existsSync(filePath)) {
    fs.unlinkSync(filePath);
    return res.json({ success: true });
  }
  res.status(404).json({ error: '文件不存在' });
});

// ========== Join Requests ==========
router.get('/join-requests', adminAuth, (req, res) => {
  const all = [...db.groupJoinRequests].sort((a, b) => b.created_at - a.created_at);
  const enriched = all.map(r => {
    const user = db.findUserById(r.from_user_id);
    const chat = db.getChat(r.chat_id);
    return {
      ...r,
      userName: user ? user.nickname : '未知',
      userUid: user ? user.uid : '',
      chatName: chat ? chat.name : '未知群组',
    };
  });
  res.json({ joinRequests: enriched });
});

module.exports = router;
